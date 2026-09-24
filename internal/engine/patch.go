package engine

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/patch"
)

// FileSet is the desired file states a Patch derived, each bound to the
// Basis (source fingerprint) it was derived from. It is opaque on purpose:
// there is no way to take the desired contents out without their Basis,
// so a stale-write guard can never be stripped before commit.
type FileSet struct {
	files []desiredFile
}

// desiredFile is one derived state: the workspace file the diff named,
// the desired bytes and permission bits (0 leaves the mode alone), and the
// observed source identity they derive from.
type desiredFile struct {
	target   workspaceFile
	contents []byte
	mode     fs.FileMode
	basis    fingerprint.FingerprintValue
}

// Patch errors. Every unsupported form wraps ErrPatchUnsupported.
var (
	// ErrPatchMalformed is a diff that is not a readable unified diff.
	ErrPatchMalformed = patch.ErrMalformed
	// ErrPatchDoesNotApply is a diff whose context does not match the
	// current source, a creation over an existing file, or a modification
	// of a missing one.
	ErrPatchDoesNotApply = patch.ErrDoesNotApply
	// ErrPatchUnsupported is any patch form that cannot reduce to a
	// desired regular-file state.
	ErrPatchUnsupported = patch.ErrUnsupported
	// ErrPatchDeleteUnsupported is a patch that deletes a file.
	ErrPatchDeleteUnsupported = patch.ErrDeleteUnsupported
	// ErrPatchRenameUnsupported is a patch that renames or copies a file.
	ErrPatchRenameUnsupported = patch.ErrRenameUnsupported
	// ErrPatchBinaryUnsupported is a binary patch.
	ErrPatchBinaryUnsupported = patch.ErrBinaryUnsupported
)

// Patch derives the desired file states a unified text diff describes,
// without mutating anything. ctx must come from a Task's Define callback.
// Each referenced source is read once under its own read claim, and the
// Basis recorded for it is the identity of exactly the bytes the hunks
// were applied to (an absent file's Basis is its observed absence).
func Patch(ctx context.Context, diff []byte) (FileSet, error) {
	task, err := beginOperation(ctx, "Patch")
	if err != nil {
		return FileSet{}, err
	}
	return task.out.derivePatch(ctx, diff)
}

func (o *Output) derivePatch(ctx context.Context, diff []byte) (FileSet, error) {
	edits, parseErr := patch.Parse(diff)
	if parseErr != nil {
		return FileSet{}, parseErr
	}
	files := make([]desiredFile, 0, len(edits))
	for _, edit := range edits {
		desired, deriveErr := o.deriveFile(ctx, edit)
		if deriveErr != nil {
			return FileSet{}, fmt.Errorf("evo: Patch %q: %w", edit.Path, deriveErr)
		}
		files = append(files, desired)
	}
	return FileSet{files: files}, nil
}

// deriveFile observes edit's source under a read claim on its path and
// applies edit to exactly the bytes observed.
func (o *Output) deriveFile(ctx context.Context, edit patch.File) (desiredFile, error) {
	target := workspaceFile{root: o.workspace(), rel: edit.Path}
	path := target.path()
	var source observedSource
	claimErr := o.holdResource(ctx, FSResource(path), resourceRead, func(context.Context) error {
		if parentErr := target.checkParents(o.fileFS()); parentErr != nil {
			return parentErr
		}
		observed, observeErr := observeSource(o.fileFS(), path)
		source = observed
		return observeErr
	})
	if claimErr != nil {
		return desiredFile{}, claimErr
	}
	switch {
	case edit.Create && source.exists:
		return desiredFile{}, fmt.Errorf("%w: creates %s, which already exists", ErrPatchDoesNotApply, path)
	case !edit.Create && !source.exists:
		return desiredFile{}, fmt.Errorf("%w: modifies %s, which does not exist", ErrPatchDoesNotApply, path)
	}
	contents, applyErr := edit.Apply(source.contents)
	if applyErr != nil {
		return desiredFile{}, applyErr
	}
	return desiredFile{target: target, contents: contents, mode: edit.Mode, basis: source.basis}, nil
}

// observedSource is one patch source as read: whether it exists, its
// bytes, and the fingerprint of exactly those bytes.
type observedSource struct {
	exists   bool
	contents []byte
	basis    fingerprint.FingerprintValue
}

// observeSource reads path read-only. Only a regular file or an absent
// path can be a patch source; a symlink or other type fails as it does for
// File.
func observeSource(fsys FileFS, path string) (observedSource, error) {
	_, exists, inspectErr := inspectFilePath(fsys, path)
	if inspectErr != nil {
		return observedSource{}, inspectErr
	}
	if !exists {
		return observedSource{basis: fingerprint.ObservedMissing(path)}, nil
	}
	contents, readErr := fsys.ReadFile(path)
	if readErr != nil {
		return observedSource{}, fmt.Errorf("evo: Patch read %q: %w", path, readErr)
	}
	return observedSource{exists: true, contents: contents, basis: fingerprint.ObservedFile(path, contents)}, nil
}
