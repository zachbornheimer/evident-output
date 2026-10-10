package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/patch"
	"github.com/zachbornheimer/evident-output/internal/publish"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// defaultFileMode is the permission a new file gets when the patch does
// not name a mode.
const defaultFileMode fs.FileMode = 0o644

// ErrPatchUnsafePath is a diff path outside the workspace, or one that
// reaches it through a symlinked directory.
var ErrPatchUnsafePath = patch.ErrUnsafePath

// ErrPatchStale is a file that changed between Patch reading it and
// committing: the concurrent edit is never overwritten.
var ErrPatchStale = errors.New("evo: Patch source changed before commit")

// patchStep is one commit a Patch plans: write a file (desired set) or
// remove one (removal set).
type patchStep struct {
	write  *desiredFile
	remove *plannedRemoval
}

// plannedRemoval is a file Patch deletes, bound to the source identity the
// deletion was validated against.
type plannedRemoval struct {
	target workspaceFile
	basis  freshness.FingerprintValue
}

// ApplyPatch applies a unified diff directly. Every affected path is
// identified and every edit validated against the bytes read before the
// first commit; each commit then revalidates its own path under that
// path's write claim, so a concurrent edit fails the Patch instead of
// being overwritten.
func ApplyPatch(ctx context.Context, diff []byte) error {
	task, err := beginOperation(ctx, "Patch")
	if err != nil {
		return err
	}
	edits, parseErr := patch.ParseStandard(diff)
	if parseErr != nil {
		return parseErr
	}
	steps, planErr := task.out.planPatch(ctx, task.id, edits)
	if planErr != nil {
		return planErr
	}
	for _, step := range steps {
		if commitErr := task.out.commitPatchStep(ctx, task.id, step); commitErr != nil {
			if errors.Is(commitErr, ErrStaleBasis) {
				return fmt.Errorf("%w: %w", ErrPatchStale, commitErr)
			}
			return commitErr
		}
	}
	return nil
}

func (o *Output) planPatch(ctx context.Context, taskID string, edits []patch.File) ([]patchStep, error) {
	var steps []patchStep
	for _, edit := range edits {
		planned, err := o.planEdit(ctx, taskID, edit)
		if err != nil {
			return nil, fmt.Errorf("evo: Patch %q: %w", edit.Path, err)
		}
		steps = append(steps, planned...)
	}
	return steps, nil
}

// planEdit turns one file's edit into commits, in an order that never
// loses data: a rename writes the destination before removing the source.
func (o *Output) planEdit(ctx context.Context, taskID string, edit patch.File) ([]patchStep, error) {
	if edit.Delete {
		return o.planDelete(ctx, edit)
	}
	if edit.From != "" {
		return o.planRename(ctx, taskID, edit)
	}
	desired, err := o.deriveStandardFile(ctx, taskID, edit)
	if err != nil {
		return nil, err
	}
	return []patchStep{{write: &desired}}, nil
}

func (o *Output) planDelete(ctx context.Context, edit patch.File) ([]patchStep, error) {
	target := workspaceFile{root: o.workspace(), rel: edit.Path}
	source, err := o.observeTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if !source.exists {
		return nil, fmt.Errorf("%w: deletes %s, which does not exist", ErrPatchDoesNotApply, target.path())
	}
	left, applyErr := edit.Apply(source.contents)
	if applyErr != nil {
		return nil, applyErr
	}
	if len(left) != 0 {
		return nil, fmt.Errorf("%w: deletes %s, which holds other content", ErrPatchDoesNotApply, target.path())
	}
	return []patchStep{{remove: &plannedRemoval{target: target, basis: source.basis}}}, nil
}

func (o *Output) planRename(ctx context.Context, taskID string, edit patch.File) ([]patchStep, error) {
	from := workspaceFile{root: o.workspace(), rel: edit.From}
	to := workspaceFile{root: o.workspace(), rel: edit.Path}
	source, err := o.observeTarget(ctx, from)
	if err != nil {
		return nil, err
	}
	if !source.exists {
		return nil, fmt.Errorf("%w: renames %s, which does not exist", ErrPatchDoesNotApply, from.path())
	}
	occupied, err := o.observeTarget(ctx, to)
	if err != nil {
		return nil, err
	}
	if occupied.exists {
		return nil, fmt.Errorf("%w: renames onto %s, which already exists", ErrPatchDoesNotApply, to.path())
	}
	contents, applyErr := edit.Apply(source.contents)
	if applyErr != nil {
		return nil, applyErr
	}
	mode := edit.Mode
	if mode == 0 {
		mode = source.mode
	}
	desired := desiredFile{target: to, contents: contents, mode: mode, basis: occupied.basis, edit: editRecord(to.path(), edit), standard: true}
	return []patchStep{{write: &desired}, {remove: &plannedRemoval{target: from, basis: source.basis}}}, nil
}

// deriveStandardFile is deriveFile for a create or modify under the
// standard forms: missing parents are allowed (Patch creates them).
func (o *Output) deriveStandardFile(ctx context.Context, taskID string, edit patch.File) (desiredFile, error) {
	target := workspaceFile{root: o.workspace(), rel: edit.Path}
	source, err := o.observeTarget(ctx, target)
	if err != nil {
		return desiredFile{}, err
	}
	identity := editRecord(target.path(), edit)
	contents, applyErr := desiredContents(edit, source, target.path(), func() bool {
		return o.taskLastLeft(ctx, taskID, source.basis, identity)
	})
	if applyErr != nil {
		return desiredFile{}, applyErr
	}
	return desiredFile{target: target, contents: contents, mode: edit.Mode, basis: source.basis, edit: identity, standard: true}, nil
}

// observeTarget reads target under a read claim after proving no parent is
// a symlink or a non-directory. A target that is a directory or another
// non-regular file is a patch that does not apply.
func (o *Output) observeTarget(ctx context.Context, target workspaceFile) (observedSource, error) {
	var source observedSource
	claimErr := o.holdResource(ctx, FSResource(target.path()), resourceRead, func(context.Context) error {
		if parentErr := target.checkStandardParents(o.fileFS()); parentErr != nil {
			return parentErr
		}
		observed, observeErr := observeSource(o.fileFS(), target.path())
		source = observed
		return observeErr
	})
	if errors.Is(claimErr, ErrFilePathTypeMismatch) {
		return observedSource{}, fmt.Errorf("%w: %w", ErrPatchDoesNotApply, claimErr)
	}
	return source, claimErr
}

func (o *Output) commitPatchStep(ctx context.Context, taskID string, step patchStep) error {
	if step.remove != nil {
		return o.removePlanned(ctx, taskID, *step.remove)
	}
	return o.publishPatchedFile(ctx, taskID, *step.write)
}

// publishPatchedFile stages file's bytes outside any lock, then publishes
// them through the shared destination lock. The guard revalidates the
// source the diff was validated against under that lock, so a concurrent
// File.Write or edit is never overwritten.
func (o *Output) publishPatchedFile(ctx context.Context, taskID string, file desiredFile) error {
	fsys := o.fileFS()
	path := file.target.path()
	desired := freshness.ObservedFile(path, file.contents)
	holdsResult := func(current observedSource) bool {
		return current.basis == desired && (file.mode == 0 || current.mode == file.mode.Perm())
	}
	// check validates the source and parents; it reports whether the file
	// already holds the diff's result.
	check := func() (bool, error) {
		if parentErr := file.target.checkStandardParents(fsys); parentErr != nil {
			return false, parentErr
		}
		current, observeErr := observeSource(fsys, path)
		if observeErr != nil {
			return false, observeErr
		}
		if holdsResult(current) {
			return true, nil
		}
		if current.basis != file.basis && current.basis != desired {
			return false, fmt.Errorf("%w: %w: %s", ErrPatchStale, ErrStaleBasis, path)
		}
		return false, nil
	}
	satisfied, checkErr := check()
	if checkErr != nil {
		return fmt.Errorf("evo: Patch %q: %w", file.target.rel, checkErr)
	}
	if satisfied {
		return nil
	}
	if o.DryRun() {
		o.recordLedgerEntry(taskID, record.NamedEntry("write", file.target.rel))
		return nil
	}
	mode := file.mode.Perm()
	if mode == 0 {
		if current, observeErr := observeSource(fsys, path); observeErr == nil && current.exists {
			mode = current.mode
		} else {
			mode = defaultFileMode
		}
	}
	staged, stageErr := publish.StageFile(ctx, path, mode, func(w io.Writer) error {
		_, writeErr := w.Write(file.contents)
		return writeErr
	})
	if stageErr != nil {
		return fmt.Errorf("evo: Patch %q: %w", file.target.rel, stageErr)
	}
	commitErr := staged.Commit(ctx, publish.Guard{
		Revalidate: func(context.Context, string) error {
			done, err := check()
			if err != nil {
				return err
			}
			if done {
				return publish.ErrSatisfied
			}
			return nil
		},
	})
	if commitErr != nil {
		return fmt.Errorf("evo: Patch %q: %w", file.target.rel, commitErr)
	}
	o.recordLedgerEntry(taskID, record.NamedEntry("write", file.target.rel))
	return nil
}

// removePlanned deletes a file while holding its path for writing, after
// proving it still holds exactly the bytes the deletion was validated
// against.
func (o *Output) removePlanned(ctx context.Context, taskID string, removal plannedRemoval) error {
	path := removal.target.path()
	fsys := o.fileFS()
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
	if o.DryRun() {
		if err := check(ctx, path); err != nil {
			return fmt.Errorf("evo: Patch remove %q: %w", removal.target.rel, err)
		}
	} else if err := publish.Remove(ctx, path, publish.Guard{Revalidate: check}); err != nil {
		return fmt.Errorf("evo: Patch remove %q: %w", removal.target.rel, err)
	}
	o.recordLedgerEntry(taskID, record.NamedEntry("remove", removal.target.rel))
	return nil
}
