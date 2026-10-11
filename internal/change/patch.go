// This file owns Patch: deriving the desired file states a unified diff
// describes, without mutating anything.

package change

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// patchEditBasisKind is the manifest Basis kind that records which edit a
// Patch-derived File applied, so a later Run can tell its own earlier
// result of this edit from bytes a different edit left.
const patchEditBasisKind = "patch-edit"

// editRecord is edit's identity as the Basis record its derived File
// operation carries.
func editRecord(path string, edit FileEdit) freshness.BasisRecord {
	identity := edit.Identity()
	return freshness.BasisRecord{Kind: patchEditBasisKind, Key: path, Digest: hex.EncodeToString(identity[:])}
}

// Patch derives the desired file states a unified text diff describes,
// without mutating anything. ctx must come from a Task's Define callback.
// Each referenced source is read once under its own read claim, and the
// Basis recorded for it is the identity of exactly the bytes the hunks
// were applied to (an absent file's Basis is its observed absence).
func Patch(ctx context.Context, diff []byte) (FileSet, error) {
	scope, beginErr := graph.BeginOperation[*Run](ctx, "Patch")
	if beginErr != nil {
		return FileSet{}, beginErr
	}
	return scope.Owner().derivePatch(ctx, scope.Task(), diff)
}

func (r *Run) derivePatch(ctx context.Context, id record.TaskID, diff []byte) (FileSet, error) {
	edits, parseErr := ParsePatch(diff)
	if parseErr != nil {
		return FileSet{}, parseErr
	}
	files := make([]desiredFile, 0, len(edits))
	for _, edit := range edits {
		desired, deriveErr := r.deriveFile(ctx, id, edit)
		if deriveErr != nil {
			return FileSet{}, fmt.Errorf("evo: Patch %q: %w", edit.Path, deriveErr)
		}
		files = append(files, desired)
	}
	return FileSet{files: files}, nil
}

// deriveFile observes edit's source under a read claim on its path and
// applies edit to exactly the bytes observed.
func (r *Run) deriveFile(ctx context.Context, id record.TaskID, edit FileEdit) (desiredFile, error) {
	target := workspaceFile{root: r.Workspace(), rel: edit.Path}
	path := target.path()
	var source observedSource
	claimErr := r.Hold(ctx, FSResource(path), ClaimRead, func(context.Context) error {
		fsys := r.host.FS()
		if parentErr := target.checkParents(fsys); parentErr != nil {
			return parentErr
		}
		observed, observeErr := observeSource(fsys, path)
		source = observed
		return observeErr
	})
	if claimErr != nil {
		return desiredFile{}, claimErr
	}
	identity := editRecord(path, edit)
	contents, applyErr := desiredContents(edit, source, path, func() bool {
		return r.taskLastLeft(ctx, id, source.basis, identity)
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
func desiredContents(edit FileEdit, source observedSource, path string, resultOfLastRun func() bool) ([]byte, error) {
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

// taskLastLeft reports whether id's previous Run recorded observed as the
// output an operation of edit left at observed's path: the manifest's proof
// that the bytes there are this Task's own earlier result of this same edit,
// not of a different diff that happened to leave them.
func (r *Run) taskLastLeft(ctx context.Context, id record.TaskID, observed freshness.FingerprintValue, edit freshness.BasisRecord) bool {
	store, openErr := r.host.Manifest(ctx)
	if openErr != nil {
		return false
	}
	key, _, ok := r.host.TaskManifest(id)
	if !ok {
		return false
	}
	prior, found := store.Task(key)
	if !found {
		return false
	}
	digest := hex.EncodeToString(observed.Digest[:])
	for _, op := range prior.Operations {
		if slices.Contains(op.Basis, edit) && slices.Contains(op.Outputs, freshness.OutputRecord{Kind: freshness.OutputKindFile, Path: observed.Key, Digest: digest}) {
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
	basis    freshness.FingerprintValue
}

// observeSource reads path read-only. Only a regular file or an absent
// path can be a patch source; a symlink or other type fails as it does for
// File.
func observeSource(fsys sysfs.FS, path string) (observedSource, error) {
	info, exists, inspectErr := inspectFilePath(fsys, path)
	if inspectErr != nil {
		return observedSource{}, inspectErr
	}
	if !exists {
		return observedSource{basis: freshness.ObservedMissing(path)}, nil
	}
	contents, readErr := fsys.ReadFile(path)
	if readErr != nil {
		return observedSource{}, fmt.Errorf("evo: Patch read %q: %w", path, readErr)
	}
	return observedSource{exists: true, contents: contents, mode: info.Mode().Perm(), basis: freshness.ObservedFile(path, contents)}, nil
}
