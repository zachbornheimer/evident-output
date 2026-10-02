package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
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
// the desired bytes and permission bits (0 leaves the mode alone), the
// observed source identity they derive from, and the identity of the edit
// that derived them.
type desiredFile struct {
	target   workspaceFile
	contents []byte
	mode     fs.FileMode
	basis    fingerprint.FingerprintValue
	edit     manifest.BasisRecord
	standard bool // planned by ApplyPatch: missing parents are created
}

// patchEditBasisKind is the manifest Basis kind that records which edit a
// Patch-derived File applied, so a later Run can tell its own earlier
// result of this edit from bytes a different edit left.
const patchEditBasisKind = "patch-edit"

// editRecord is edit's identity as the Basis record its derived File
// operation carries.
func editRecord(path string, edit patch.File) manifest.BasisRecord {
	identity := edit.Identity()
	return manifest.BasisRecord{Kind: patchEditBasisKind, Key: path, Digest: hex.EncodeToString(identity[:])}
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
	return task.out.derivePatch(ctx, task.id, diff)
}

func (o *Output) derivePatch(ctx context.Context, taskID string, diff []byte) (FileSet, error) {
	edits, parseErr := patch.Parse(diff)
	if parseErr != nil {
		return FileSet{}, parseErr
	}
	files := make([]desiredFile, 0, len(edits))
	for _, edit := range edits {
		desired, deriveErr := o.deriveFile(ctx, taskID, edit)
		if deriveErr != nil {
			return FileSet{}, fmt.Errorf("evo: Patch %q: %w", edit.Path, deriveErr)
		}
		files = append(files, desired)
	}
	return FileSet{files: files}, nil
}

// deriveFile observes edit's source under a read claim on its path and
// applies edit to exactly the bytes observed.
func (o *Output) deriveFile(ctx context.Context, taskID string, edit patch.File) (desiredFile, error) {
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
	identity := editRecord(path, edit)
	contents, applyErr := desiredContents(edit, source, path, func() bool {
		return o.taskLastLeft(ctx, taskID, source.basis, identity)
	})
	if applyErr != nil {
		return desiredFile{}, applyErr
	}
	return desiredFile{target: target, contents: contents, mode: edit.Mode, basis: source.basis, edit: identity}, nil
}

// desiredContents is the bytes edit leaves at source. The diff is applied
// forward first, as patch(1) and git apply do. A source that already
// holds edit's result (the same diff on a second Run) is its own desired
// state, which Files then reports as already satisfied: when the forward
// apply fails and the reverse matches (patch -N), or when source matches
// both sides and resultOfLastRun confirms these bytes are what this
// Task's previous Run left there by applying this same edit.
func desiredContents(edit patch.File, source observedSource, path string, resultOfLastRun func() bool) ([]byte, error) {
	applied := source.exists && edit.AppliedTo(source.contents)
	switch {
	case !edit.Create && !source.exists:
		return nil, fmt.Errorf("%w: modifies %s, which does not exist", ErrPatchDoesNotApply, path)
	case edit.Create && source.exists && !applied:
		return nil, fmt.Errorf("%w: creates %s, which already exists", ErrPatchDoesNotApply, path)
	case edit.Create && applied:
		return source.contents, nil
	}
	forward, applyErr := edit.Apply(source.contents)
	switch {
	case applyErr != nil && applied && errors.Is(applyErr, ErrPatchDoesNotApply):
		return source.contents, nil
	case applyErr != nil:
		return nil, applyErr
	case applied && resultOfLastRun():
		return source.contents, nil
	}
	return forward, nil
}

// taskLastLeft reports whether taskID's previous Run recorded observed as
// the output an operation of edit left at observed's path: the manifest's
// proof that the bytes there are this Task's own earlier result of this
// same edit, not of a different diff that happened to leave them.
func (o *Output) taskLastLeft(ctx context.Context, taskID string, observed fingerprint.FingerprintValue, edit manifest.BasisRecord) bool {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return false
	}
	o.mu.Lock()
	key, _, ok := o.taskManifestKeyLocked(taskID)
	o.mu.Unlock()
	if !ok {
		return false
	}
	prior, found := store.Task(key)
	if !found {
		return false
	}
	digest := hex.EncodeToString(observed.Digest[:])
	for _, op := range prior.Operations {
		if slices.Contains(op.Basis, edit) && slices.Contains(op.Outputs, manifest.OutputRecord{Kind: "file", Path: observed.Key, Digest: digest}) {
			return true
		}
	}
	return false
}

// observedSource is one patch source as read: whether it exists, its
// bytes, and the fingerprint of exactly those bytes.
type observedSource struct {
	exists   bool
	contents []byte
	mode     fs.FileMode
	basis    fingerprint.FingerprintValue
}

// observeSource reads path read-only. Only a regular file or an absent
// path can be a patch source; a symlink or other type fails as it does for
// File.
func observeSource(fsys FileFS, path string) (observedSource, error) {
	info, exists, inspectErr := inspectFilePath(fsys, path)
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
	return observedSource{exists: true, contents: contents, mode: info.Mode().Perm(), basis: fingerprint.ObservedFile(path, contents)}, nil
}
