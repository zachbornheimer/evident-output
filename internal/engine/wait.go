package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// runWaitedWork executes, on the waiting caller's own goroutine, the work
// standing between it and the task it awaits (see graph.Graph.RunWaited).
func (o *Output) runWaitedWork(taskID string, stack *graph.WaiterStack) {
	o.mu.Lock()
	st := o.taskStates[taskID]
	o.mu.Unlock()
	if st != nil {
		o.graph.RunWaited(st.node, stack)
	}
}

// sealAwaitedInputs seals everything the awaited Task waits for (see
// graph.Graph.SealAwaited). A nil seen walks fresh.
func (o *Output) sealAwaitedInputs(taskID string, seen *graph.InputSeals) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskStates[taskID]; st != nil {
		o.graph.SealAwaited(st.node, seen)
	}
}

// Wait blocks until the task is terminal and returns the error its callback
// returned (nil on Done, Skipped, or a dry run that never invoked it).
//
// A task that was already resolved before it was defined never runs its
// callback (the misuse is recorded at the Define call), and Wait returns nil
// immediately rather than blocking on work that will never happen (P15).
//
// A task that never ran — an abandoned queue, a predecessor that failed —
// returns ErrNotStarted rather than nil, and a cancelled one returns its
// cancellation: Wait never reports success for work that did not happen.
//
// Wait called while the calling goroutine holds a resource claim (inside an
// Effect, File, or Basis hold) returns ErrNestedResourceAcquisition without
// waiting: the awaited work could need that claim, and neither could move.
//
// A waiter has stopped doing work, so the concurrency ceiling must not be
// the reason the task it waits on cannot start (P16). Wait therefore runs
// that task, and whatever is holding it back, on its own goroutine when the
// scheduler has not picked them up (see runWaitedWork): nested Define+Wait
// completes even at MaxConcurrency 1, because the waiting callback's slot
// carries the work it is waiting for instead of idling.
//
// MaxConcurrency bounds every executing callback. A plain caller (one
// outside any callback) holds no slot, so it runs work only in a free slot
// and otherwise waits for the pool. A goroutine a callback started holds
// none either: if that callback blocks on it while every slot is held,
// neither can move, and Wait returns ErrWaitDeadlock naming the shape
// instead of hanging. Declare that work as Tasks under a Group, or Wait
// inside the callback itself (API-041).
func (t *TaskHandle) Wait() error {
	if t == nil || t.out == nil {
		return nil
	}
	if t.rejected != nil {
		return rejectedWaitOutcome(t.rejected)
	}
	var stack graph.WaiterStack
	if err := t.out.refuseWaitUnderClaim(t.id, &stack); err != nil {
		return err
	}
	return t.waitChecked(&stack, nil)
}

// waitChecked is Wait after its caller already refused a held claim. stack
// and seen are shared by every Task a Group or Sequence Wait waits on (see
// waitDescendants), so one Wait walks its stack at most once, and not at
// all unless a claim is held somewhere or it actually parks, and walks
// shared inputs once.
func (t *TaskHandle) waitChecked(stack *graph.WaiterStack, seen *graph.InputSeals) error {
	t.out.sealAwaitedInputs(t.id, seen)
	t.out.runWaitedWork(t.id, stack)
	if err := t.waitSubmitted(stack); err != nil {
		return err
	}
	return t.out.waitOutcome(t.id)
}

// waitOutcome is the truth Wait owes its caller: the error the callback
// returned, or — when the callback never ran at all — the reason it did not.
// Answering with the zero value of "what the callback returned" is how a
// waiter came to resolve Done directly above the row admitting the work it
// awaited never started.
func (o *Output) waitOutcome(taskID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	switch {
	case st == nil:
		return nil
	case st.node.WorkErr() != nil:
		return st.node.WorkErr()
	case st.node.Phase() == graph.PhaseDeclared && (st.node.Rec.State() == NotStarted || st.neverDefined()):
		// Declared but never Defined: there is no work to have succeeded.
		return fmt.Errorf("%w: %s was never defined", ErrNotStarted, st.name)
	case st.node.Rec.State() == NotStarted:
		return ErrNotStarted
	case st.node.Rec.State() == Cancelled:
		return cancelledWaitOutcome(st.node.Rec.Summary())
	case st.node.Rec.State() == Failed || st.node.Rec.State() == Blocked:
		// The row already failed but its callback has not returned yet (it
		// resolved itself via Fail/Block, which settles the row at once),
		// or it returned nil after stating its own failure. Either way the
		// work did not succeed, and Wait must not say it did.
		return failedWaitOutcome(st.node.Rec.Summary())
	default:
		return nil
	}
}

// cancelledWaitOutcome carries the reason the cancelled row already shows
// into the waiter's error, so the caller's own message and the ledger say
// the same thing.
func cancelledWaitOutcome(reason string) error {
	if reason == "" {
		return errWaitCancelled
	}
	return fmt.Errorf("%w: %s", errWaitCancelled, reason)
}

// failedWaitOutcome is cancelledWaitOutcome's counterpart for a row that
// resolved Failed or Blocked without (yet) a recorded callback error.
func failedWaitOutcome(summary string) error {
	if summary == "" {
		return errWaitFailed
	}
	return fmt.Errorf("%w: %s", errWaitFailed, summary)
}

// waitSubmitted parks until the task resolves. A non-nil error means the
// scheduler proved the wait could never be satisfied and released the
// caller instead of letting it hang (see graph.Graph.Kick).
func (t *TaskHandle) waitSubmitted(stack *graph.WaiterStack) error {
	if t == nil || t.out == nil {
		return nil
	}
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	if st == nil || st.neverDefined() {
		o.mu.Unlock()
		return nil
	}
	// A terminal task closed its doneCh in the same critical section that
	// set its state, so there is nothing to park for.
	if core.IsTerminalTask(st.node.Rec.State()) {
		o.mu.Unlock()
		return nil
	}
	ch := st.node.Done()
	o.mu.Unlock()
	ticket := o.graph.BeginWait(st.node, stack.CallbackDepth())
	defer o.graph.EndWait(ticket)
	select {
	case <-ch:
		return nil
	case <-ticket.Aborted():
		return ticket.Released()
	}
}
