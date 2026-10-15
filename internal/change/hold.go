// This file owns how a Run holds a claim: the process-wide registry, the
// frame every granted claim's work runs beneath, and the waiting activity a
// contended claim shows on its Task.

package change

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// processClaims coordinates every Run in this process: two Runs touching the
// same file must exclude each other just as two Tasks in one Run do.
var processClaims = NewClaimRegistry()

// Hold holds res in mode for fn's duration and releases it however fn ends.
// It is the only way a Run acquires a claim; there is no lock/unlock pair to
// misuse. A ctx that already holds a claim fails with
// ErrNestedResourceAcquisition before any wait.
//
// An uncontended claim is invisible. When the claim has to wait and ctx
// belongs to a Task's Define callback, that Task shows "waiting for <res>"
// as its live activity until the claim is granted.
func (r *Run) Hold(ctx context.Context, res Resource, mode ClaimMode, fn func(context.Context) error) error {
	wait := r.waitFor(ctx, res)
	defer wait.clear()
	req := ClaimRequest{Resource: res, Workspace: r.Workspace(), Mode: mode, OnContended: wait.show}
	return processClaims.HoldResource(ctx, req, func(held context.Context) error {
		wait.clear()
		return runHoldingResource(held, fn)
	})
}

// Run holds the read claims freshness observes Basis under.
var _ freshness.Claimer = (*Run)(nil)

// HoldRead runs observe holding a read claim on path's filesystem resource.
// It is what freshness asks of its Claimer.
func (r *Run) HoldRead(ctx context.Context, path string, observe func(context.Context) error) error {
	return r.Hold(ctx, FSResource(path), ClaimRead, observe)
}

// runHoldingResource is the single frame every granted claim's work runs
// beneath, so Wait can tell from its own goroutine's stack that the caller
// holds a claim, and from the hold sensor (graph.Holds) that the goroutine
// which started it does (see graph.Graph.Wait).
func runHoldingResource(held context.Context, fn func(context.Context) error) error {
	defer graph.ProcessHolds().Enter().Leave()
	return fn(held)
}

// checkResourceFree fails with ErrNestedResourceAcquisition when ctx already
// holds a resource. Operations that may block before claiming their own
// resource (opening the cross-process manifest lock) call it first, so a held
// claim can never wait on anything else.
func checkResourceFree(ctx context.Context, res Resource, mode ClaimMode) error {
	return CheckClaimFree(ctx, fmt.Sprintf("%s %v", mode, res))
}

// validateResource resolves res without claiming it, so a dry run rejects an
// invalid Resource exactly like an applied run does.
func (r *Run) validateResource(res Resource) error {
	if _, err := ResolveResource(res, r.Workspace()); err != nil {
		return fmt.Errorf("evo: resource %v: %w", res, err)
	}
	return nil
}

// claimWait is one claim's waiting activity: shown on its Task only if the
// claim is contended, and cleared the moment the claim is granted (or
// abandoned). show and clear run on the claiming goroutine.
type claimWait struct {
	run     *Run
	task    record.TaskID
	label   string
	restore func()
}

// waitFor prepares res's waiting activity for the Task ctx belongs to.
// Outside this Run's Define callbacks there is no row to show it on, and
// show does nothing.
func (r *Run) waitFor(ctx context.Context, res Resource) *claimWait {
	wait := &claimWait{run: r, label: ResourceLabel(res)}
	if scope, err := graph.ScopeOf[*Run](ctx); err == nil && scope.Owner() == r {
		wait.task = scope.Task()
	}
	return wait
}

func (w *claimWait) show(Claim) {
	if w.task == "" {
		return
	}
	w.restore = w.run.host.ShowWaiting(w.task, w.label)
}

// clear restores the Task's previous activity, unless something else
// replaced the waiting text in the meantime.
func (w *claimWait) clear() {
	if w.restore == nil {
		return
	}
	restore := w.restore
	w.restore = nil
	restore()
}
