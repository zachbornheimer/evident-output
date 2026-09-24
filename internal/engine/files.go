package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
)

// ErrStaleBasis is returned when a derived file's source changed after its
// Basis was observed: committing would overwrite state the derivation
// never saw.
var ErrStaleBasis = errors.New("evo: source changed since its Basis was observed")

// Files commits each desired state in files through File, in the order
// Patch derived them. ctx must come from a Task's Define callback.
//
// Each file's source Basis is revalidated while File holds that path for
// writing, immediately before it commits; a source changed since
// derivation fails with ErrStaleBasis and is never overwritten. Files is
// not a transaction: it stops at the first failing file, and files it
// already committed keep their Effects.
func Files(ctx context.Context, files FileSet) error {
	task, err := beginOperation(ctx, "Files")
	if err != nil {
		return err
	}
	for _, file := range files.files {
		if commitErr := task.out.establishFile(ctx, file.operation(task.id)); commitErr != nil {
			return fmt.Errorf("evo: Files %q: %w", file.displayPath, commitErr)
		}
	}
	return nil
}

// operation is the File operation that establishes f for taskID, carrying
// the source f was derived from.
func (f desiredFile) operation(taskID string) fileOperation {
	return fileOperation{
		taskID:      taskID,
		spec:        FileSpec{Path: f.displayPath, Contents: f.contents, Mode: f.mode},
		path:        f.path,
		derivedFrom: &derivation{basis: f.basis, desired: fingerprint.ObservedFile(f.path, f.contents)},
	}
}

// derivation is where a desired file state came from: the Basis its
// contents were derived from and the identity of those desired contents.
type derivation struct {
	basis   fingerprint.FingerprintValue
	desired fingerprint.FingerprintValue
}

// revalidate observes path now and fails with ErrStaleBasis unless it is
// still the derivation's source or already holds the desired contents
// (an already-satisfied state File then leaves alone).
func (d derivation) revalidate(fsys FileFS, path string) error {
	current, observeErr := observeSource(fsys, path)
	if observeErr != nil {
		return observeErr
	}
	if current.basis != d.basis && current.basis != d.desired {
		return fmt.Errorf("%w: %s", ErrStaleBasis, path)
	}
	return nil
}
