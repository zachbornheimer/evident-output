// This file owns publishing one planned Patch step: a file written through
// the atomic publisher, or a file removed, each revalidated under the
// destination lock so a concurrent edit is never overwritten.

package change

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// patchWrite is one planned file write on its way to disk: the desired
// state, the filesystem it is checked against and the identity the file has
// once it holds the diff's result.
type patchWrite struct {
	file    desiredFile
	fsys    sysfs.FS
	path    string
	desired freshness.FingerprintValue
}

func (r *Run) newPatchWrite(file desiredFile) patchWrite {
	path := file.target.path()
	return patchWrite{file: file, fsys: r.host.FS(), path: path, desired: freshness.ObservedFile(path, file.contents)}
}

// holdsResult reports whether current already is the diff's result.
func (w patchWrite) holdsResult(current observedSource) bool {
	return current.basis == w.desired && (w.file.mode == 0 || current.mode == w.file.mode.Perm())
}

// check validates the source and parents; it reports whether the file
// already holds the diff's result.
func (w patchWrite) check() (satisfied bool, err error) {
	if parentErr := w.file.target.checkStandardParents(w.fsys); parentErr != nil {
		return false, parentErr
	}
	current, observeErr := observeSource(w.fsys, w.path)
	if observeErr != nil {
		return false, observeErr
	}
	if w.holdsResult(current) {
		return true, nil
	}
	if current.basis != w.file.basis && current.basis != w.desired {
		return false, fmt.Errorf("%w: %w: %s", ErrPatchStale, ErrStaleBasis, w.path)
	}
	return false, nil
}

// mode is the permission the written file gets: the diff's, else the
// existing file's own, else the default for a new file.
func (w patchWrite) mode() fs.FileMode {
	if mode := w.file.mode.Perm(); mode != 0 {
		return mode
	}
	if current, observeErr := observeSource(w.fsys, w.path); observeErr == nil && current.exists {
		return current.mode
	}
	return defaultFileMode
}

// publish stages the bytes outside any lock, then commits them through the
// shared destination lock. The guard revalidates under that lock, so a
// concurrent File.Write or edit is never overwritten.
func (w patchWrite) publish(ctx context.Context) error {
	staged, stageErr := StageFile(ctx, w.path, w.mode(), func(out io.Writer) error {
		_, writeErr := out.Write(w.file.contents)
		return writeErr
	})
	if stageErr != nil {
		return fmt.Errorf("evo: Patch %q: %w", w.file.target.rel, stageErr)
	}
	commitErr := staged.Commit(ctx, PublishGuard{
		Revalidate: func(context.Context, string) error {
			done, checkErr := w.check()
			if checkErr != nil {
				return checkErr
			}
			if done {
				return ErrPublishSatisfied
			}
			return nil
		},
	})
	if commitErr != nil {
		return fmt.Errorf("evo: Patch %q: %w", w.file.target.rel, commitErr)
	}
	return nil
}

// publishPatchedFile checks, then (in an applied Run) publishes file, and
// records the write.
func (r *Run) publishPatchedFile(ctx context.Context, id record.TaskID, file desiredFile) error {
	write := r.newPatchWrite(file)
	satisfied, checkErr := write.check()
	if checkErr != nil {
		return fmt.Errorf("evo: Patch %q: %w", file.target.rel, checkErr)
	}
	if satisfied {
		return nil
	}
	if !r.host.DryRun() {
		if publishErr := write.publish(ctx); publishErr != nil {
			return publishErr
		}
	}
	r.recordEntry(id, record.NamedEntry("write", file.target.rel))
	return nil
}

// removePlanned deletes a file while holding its path for writing, after
// proving it still holds exactly the bytes the deletion was validated
// against.
func (r *Run) removePlanned(ctx context.Context, id record.TaskID, removal plannedRemoval) error {
	path := removal.target.path()
	fsys := r.host.FS()
	check := func(context.Context, string) error {
		if parentErr := removal.target.checkStandardParents(fsys); parentErr != nil {
			return parentErr
		}
		current, observeErr := observeSource(fsys, path)
		switch {
		case observeErr != nil:
			return observeErr
		case current.basis != removal.basis:
			return fmt.Errorf("%w: %w: %s", ErrPatchStale, ErrStaleBasis, path)
		}
		return nil
	}
	if r.host.DryRun() {
		if checkErr := check(ctx, path); checkErr != nil {
			return fmt.Errorf("evo: Patch remove %q: %w", removal.target.rel, checkErr)
		}
	} else if removeErr := RemoveGuarded(ctx, path, PublishGuard{Revalidate: check}); removeErr != nil {
		return fmt.Errorf("evo: Patch remove %q: %w", removal.target.rel, removeErr)
	}
	r.recordEntry(id, record.NamedEntry("remove", removal.target.rel))
	return nil
}
