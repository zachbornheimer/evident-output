// This file owns Files and the FileSet a Patch derives: desired file states
// bound to the Basis they were derived from, so a stale write is refused.

package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// ErrStaleBasis is returned when a derived file's source changed after its
// Basis was observed: committing would overwrite state the derivation
// never saw.
var ErrStaleBasis = errors.New("evo: source changed since its Basis was observed")

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
	basis    freshness.FingerprintValue
	edit     freshness.BasisRecord
	standard bool // planned by ApplyPatch: missing parents are created
}

// Files commits each desired state in files through File, in the order
// Patch derived them. ctx must come from a Task's Define callback.
//
// Each file's source Basis is revalidated while File holds that path for
// writing, immediately before it commits; a source changed since
// derivation fails with ErrStaleBasis and is never overwritten. Files is
// not a transaction: it stops at the first failing file, and files it
// already committed keep their Effects.
func Files(ctx context.Context, files FileSet) error {
	scope, beginErr := graph.BeginOperation[*Run](ctx, "Files")
	if beginErr != nil {
		return beginErr
	}
	run := scope.Owner()
	for _, file := range files.files {
		if commitErr := run.establishFile(ctx, file.operation(scope.Task())); commitErr != nil {
			return fmt.Errorf("evo: Files %q: %w", file.target.rel, commitErr)
		}
	}
	return nil
}

// operation is the File operation that establishes f for id, carrying the
// source f was derived from.
func (f desiredFile) operation(id record.TaskID) fileOperation {
	path := f.target.path()
	return fileOperation{
		taskID:      id,
		spec:        FileSpec{Path: f.target.rel, Contents: f.contents, Mode: f.mode},
		path:        path,
		derivedFrom: &derivation{standard: f.standard, target: f.target, basis: f.basis, desired: freshness.ObservedFile(path, f.contents), edit: f.edit},
	}
}

// derivation is where a desired file state came from: the workspace file
// it targets, the Basis its contents were derived from, the identity of
// those desired contents, and the edit that derived them.
type derivation struct {
	standard bool
	target   workspaceFile
	basis    freshness.FingerprintValue
	desired  freshness.FingerprintValue
	edit     freshness.BasisRecord
}

// recordedBasis is basis plus d's edit identity, in canonical order: the
// Basis the derived File operation records, so the next Run can prove
// which edit left its output.
func (d derivation) recordedBasis(basis []freshness.BasisRecord) []freshness.BasisRecord {
	records := append(slices.Clone(basis), d.edit)
	freshness.SortBasisRecords(records)
	return records
}

// revalidate observes path now and fails unless every parent is still a
// real directory beneath the workspace and path is still the derivation's
// source (ErrStaleBasis otherwise) or already holds the desired contents
// (an already-satisfied state File then leaves alone).
func (d derivation) revalidate(fsys sysfs.FS, path string) error {
	parents := d.target.checkParents
	if d.standard {
		parents = d.target.checkStandardParents
	}
	if parentErr := parents(fsys); parentErr != nil {
		return parentErr
	}
	current, observeErr := observeSource(fsys, path)
	if observeErr != nil {
		return observeErr
	}
	if current.basis != d.basis && current.basis != d.desired {
		return fmt.Errorf("%w: %s", ErrStaleBasis, path)
	}
	return nil
}
