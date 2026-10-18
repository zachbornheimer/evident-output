// This file owns ApplyPatch: planning every affected path of a unified diff,
// then publishing each result atomically under its path's write claim.

package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// defaultFileMode is the permission a new file gets when the patch does
// not name a mode.
const defaultFileMode fs.FileMode = 0o644

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
	scope, beginErr := graph.BeginOperation[*Run](ctx, "Patch")
	if beginErr != nil {
		return beginErr
	}
	edits, parseErr := ParseStandardPatch(diff)
	if parseErr != nil {
		return parseErr
	}
	run, id := scope.Owner(), scope.Task()
	steps, planErr := run.planPatch(ctx, id, edits)
	if planErr != nil {
		return planErr
	}
	for _, step := range steps {
		if commitErr := run.commitPatchStep(ctx, id, step); commitErr != nil {
			if errors.Is(commitErr, ErrStaleBasis) {
				return fmt.Errorf("%w: %w", ErrPatchStale, commitErr)
			}
			return commitErr
		}
	}
	return nil
}

func (r *Run) planPatch(ctx context.Context, id record.TaskID, edits []FileEdit) ([]patchStep, error) {
	var steps []patchStep
	for _, edit := range edits {
		planned, err := r.planEdit(ctx, id, edit)
		if err != nil {
			return nil, fmt.Errorf("evo: Patch %q: %w", edit.Path, err)
		}
		steps = append(steps, planned...)
	}
	return steps, nil
}

// planEdit turns one file's edit into commits, in an order that never
// loses data: a rename writes the destination before removing the source.
func (r *Run) planEdit(ctx context.Context, id record.TaskID, edit FileEdit) ([]patchStep, error) {
	if edit.Delete {
		return r.planDelete(ctx, edit)
	}
	if edit.From != "" {
		return r.planRename(ctx, id, edit)
	}
	desired, err := r.deriveStandardFile(ctx, id, edit)
	if err != nil {
		return nil, err
	}
	return []patchStep{{write: &desired}}, nil
}

func (r *Run) planDelete(ctx context.Context, edit FileEdit) ([]patchStep, error) {
	target := workspaceFile{root: r.Workspace(), rel: edit.Path}
	source, err := r.observeTarget(ctx, target)
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

func (r *Run) planRename(ctx context.Context, id record.TaskID, edit FileEdit) ([]patchStep, error) {
	from := workspaceFile{root: r.Workspace(), rel: edit.From}
	to := workspaceFile{root: r.Workspace(), rel: edit.Path}
	source, err := r.observeTarget(ctx, from)
	if err != nil {
		return nil, err
	}
	if !source.exists {
		return nil, fmt.Errorf("%w: renames %s, which does not exist", ErrPatchDoesNotApply, from.path())
	}
	occupied, err := r.observeTarget(ctx, to)
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
func (r *Run) deriveStandardFile(ctx context.Context, id record.TaskID, edit FileEdit) (desiredFile, error) {
	target := workspaceFile{root: r.Workspace(), rel: edit.Path}
	source, err := r.observeTarget(ctx, target)
	if err != nil {
		return desiredFile{}, err
	}
	identity := editRecord(target.path(), edit)
	contents, applyErr := desiredContents(edit, source, target.path(), func() bool {
		return r.taskLastLeft(ctx, id, source.basis, identity)
	})
	if applyErr != nil {
		return desiredFile{}, applyErr
	}
	return desiredFile{target: target, contents: contents, mode: edit.Mode, basis: source.basis, edit: identity, standard: true}, nil
}

// observeTarget reads target under a read claim after proving no parent is
// a symlink or a non-directory. A target that is a directory or another
// non-regular file is a patch that does not apply.
func (r *Run) observeTarget(ctx context.Context, target workspaceFile) (observedSource, error) {
	var source observedSource
	claimErr := r.Hold(ctx, FSResource(target.path()), ClaimRead, func(context.Context) error {
		fsys := r.host.FS()
		if parentErr := target.checkStandardParents(fsys); parentErr != nil {
			return parentErr
		}
		observed, observeErr := observeSource(fsys, target.path())
		source = observed
		return observeErr
	})
	if errors.Is(claimErr, ErrFilePathTypeMismatch) {
		return observedSource{}, fmt.Errorf("%w: %w", ErrPatchDoesNotApply, claimErr)
	}
	return source, claimErr
}

func (r *Run) commitPatchStep(ctx context.Context, id record.TaskID, step patchStep) error {
	if step.remove != nil {
		return r.removePlanned(ctx, id, *step.remove)
	}
	return r.publishPatchedFile(ctx, id, *step.write)
}
