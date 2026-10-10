package graph

import (
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/record"
)

var (
	// ErrNotStarted is what a wait returns for a Task whose work never ran: a
	// failed or abandoned predecessor, a run that drained before the Task
	// became eligible, or a Task nobody ever Defined.
	ErrNotStarted = errors.New("evo: awaited task never started")
	// ErrWaitCancelled is what a wait returns for a Task an interrupt or an
	// explicit Cancel resolved instead of running.
	ErrWaitCancelled = errors.New("evo: awaited task was cancelled")
	// ErrWaitFailed is what a wait returns for a row that resolved Failed or
	// Blocked with no callback error recorded: the callback settled its own
	// row and has not returned yet, or returned nil after doing so.
	ErrWaitFailed = errors.New("evo: awaited task did not succeed")
	// ErrWaitUnderClaim is why a wait was refused, unless the graph was built
	// with WithWaitUnderClaimError: the caller holds a resource claim, and the
	// awaited work could need it.
	ErrWaitUnderClaim = errors.New("evo: wait while holding a resource claim")
)

// WithWaitUnderClaimError makes err the sentinel a wait refused for a held
// resource claim wraps, so the caller's own public error is what the waiter
// matches.
func WithWaitUnderClaimError(err error) Option { return func(g *Graph) { g.waitUnderClaim = err } }

// Wait blocks until t is terminal and returns the error its callback
// returned: nil on Done, Skipped, or a dry run that never invoked it.
//
// A Task that never ran returns ErrNotStarted rather than nil, and a
// cancelled one returns its cancellation: Wait never reports success for work
// that did not happen. A caller holding a resource claim is refused without
// waiting, because the awaited work could need that claim and neither could
// move.
//
// A waiter has stopped doing work, so the concurrency ceiling must not be the
// reason the Task it waits on cannot start: Wait runs that Task, and whatever
// holds it back, on the caller's own goroutine when the scheduler has not
// picked them up (see RunWaited). A goroutine a callback started holds no
// slot: if that callback blocks on it while every slot is held, neither can
// move, and Wait returns ErrWaitDeadlock naming the shape instead of hanging.
func (g *Graph) Wait(t *Task) error {
	var stack WaiterStack
	if err := g.refuseUnderClaim(t.Name, &stack); err != nil {
		return err
	}
	return g.waitChecked(t, &stack, nil)
}

// refuseUnderClaim refuses a wait on the Task or container named name when
// the calling goroutine, or the goroutine that started it, holds a resource
// claim. Waiting under a claim is nested acquisition in disguise, so it is
// refused every time, not only when it would conflict: the outcome never
// depends on timing. Wait takes no context, so the claim is read from the
// stack (see Holds).
func (g *Graph) refuseUnderClaim(name string, stack *WaiterStack) error {
	if !stack.HoldsClaim() {
		return nil
	}
	return fmt.Errorf("%w: Wait on %q while holding a resource claim", g.waitUnderClaim, name)
}

// waitChecked is Wait after its caller already refused a held claim. stack
// and seen are shared by every Task a container Wait waits on, so one Wait
// walks its stack at most once, and not at all unless a claim is held
// somewhere or it actually parks, and walks shared inputs once.
func (g *Graph) waitChecked(t *Task, stack *WaiterStack, seen *InputSeals) error {
	g.SealAwaited(t, seen)
	g.RunWaited(t, stack)
	if err := g.parkUntilSettled(t, stack); err != nil {
		return err
	}
	return g.waitOutcome(t)
}

// parkUntilSettled parks until t resolves. A non-nil error means the
// scheduler proved the wait could never be satisfied and released the caller
// instead of letting it hang (see Kick).
func (g *Graph) parkUntilSettled(t *Task, stack *WaiterStack) error {
	g.lockRead()
	settledOrOwed := terminal(t) || t.neverDefinedLocked()
	g.unlockRead()
	if settledOrOwed {
		return nil
	}
	ticket := g.BeginWait(t, stack.CallbackDepth())
	defer g.EndWait(ticket)
	select {
	case <-t.done:
		return nil
	case <-ticket.Aborted():
		return ticket.Released()
	}
}

// waitOutcome is the truth Wait owes its caller: the error the callback
// returned, or, when the callback never ran at all, the reason it did not.
// Answering with the zero value of "what the callback returned" is how a
// waiter came to resolve Done directly above the row admitting the work it
// awaited never started.
func (g *Graph) waitOutcome(t *Task) error {
	g.lockRead()
	defer g.unlockRead()
	state := t.Rec.State()
	switch {
	case t.workErr != nil:
		return t.workErr
	case t.sched.phase == PhaseDeclared && (state == record.NotStarted || t.neverDefinedLocked()):
		// Declared but never Defined: there is no work to have succeeded.
		return fmt.Errorf("%w: %s was never defined", ErrNotStarted, t.Name)
	case state == record.NotStarted:
		return ErrNotStarted
	case state == record.Cancelled:
		return withReason(ErrWaitCancelled, t.Rec.Summary())
	case state == record.Failed || state == record.Blocked:
		// The row already failed but its callback has not returned yet (it
		// resolved itself via Fail/Block, which settles the row at once),
		// or it returned nil after stating its own failure. Either way the
		// work did not succeed, and Wait must not say it did.
		return withReason(ErrWaitFailed, t.Rec.Summary())
	default:
		return nil
	}
}

// withReason carries the reason the row already shows into the waiter's
// error, so the caller's own message and the ledger say the same thing.
func withReason(sentinel error, reason string) error {
	if reason == "" {
		return sentinel
	}
	return fmt.Errorf("%w: %s", sentinel, reason)
}
