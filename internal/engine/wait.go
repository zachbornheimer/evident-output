package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// waitTicket is one goroutine's registration while it is parked in
// TaskHandle.Wait: the task it awaits, how many task callbacks it is
// holding still on its own stack, and the channel the scheduler closes to
// release it when it proves the wait can never be satisfied.
type waitTicket struct {
	taskID string
	depth  int
	abort  chan struct{}
}

// beginWait registers this goroutine's park and re-tests the run: a newly
// parked waiter may be the last thing that could have moved it.
func (o *Output) beginWait(taskID string, depth int) *waitTicket {
	ticket := &waitTicket{taskID: taskID, depth: depth, abort: make(chan struct{})}
	o.mu.Lock()
	if o.sched.waits == nil {
		o.sched.waits = make(map[*waitTicket]struct{})
	}
	o.sched.waits[ticket] = struct{}{}
	o.mu.Unlock()
	o.kick()
	return ticket
}

func (o *Output) endWait(ticket *waitTicket) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.sched.waits, ticket)
}

// runWaitedWork executes, on the waiting caller's own goroutine, the work
// standing between it and the task it awaits: that task itself when the
// scheduler has not started it, and otherwise whatever the scheduler is
// holding back that could make it eligible.
//
// A waiter has stopped doing work, so the concurrency ceiling must never be
// the reason the task it waits on cannot start (P16). Donating only to the
// awaited task was not enough: at MaxConcurrency 1 the waiter's own slot can
// be the only thing keeping that task's unmet predecessor queued, so the
// wait ended only when the run drained and abandoned the whole chain.
//
// The loop terminates: every donation resolves one task, and a resolved task
// is never claimable again.
func (o *Output) runWaitedWork(taskID string) {
	for {
		if o.runForWaiter(taskID) {
			return
		}
		if !o.awaitedWorkIsStalled(taskID) {
			return
		}
		if !o.runOneStalledTask() {
			return
		}
	}
}

// awaitedWorkIsStalled reports whether the awaited task is submitted work
// that no goroutine is executing — the only case where the waiter going to
// sleep is itself what prevents progress. A task another goroutine is
// already running, or one already resolved, needs no donation.
func (o *Output) awaitedWorkIsStalled(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	return st != nil && st.awaitingStart()
}

// runForWaiter executes the task a caller is about to block on, on the
// caller's own goroutine, when the scheduler has not started it yet, and
// reports whether it did. A no-op when the task is not claimable — already
// running, already terminal, never defined, not yet eligible, or the run is
// cancelling.
func (o *Output) runForWaiter(taskID string) bool {
	st, fn, claimed := o.claimForWaiter(taskID)
	if !claimed {
		return false
	}
	o.executeClaimed(st, fn)
	return true
}

// runOneStalledTask executes one task the scheduler has room for nobody to
// start, on the waiting caller's own goroutine, and reports whether it found
// one (see runWaitedWork).
func (o *Output) runOneStalledTask() bool {
	st, fn, claimed := o.claimAnyForWaiter()
	if !claimed {
		return false
	}
	o.executeClaimed(st, fn)
	return true
}

// claimForWaiter marks one named submitted, eligible, not-yet-started task
// as running for a waiting goroutine.
func (o *Output) claimForWaiter(taskID string) (st *taskState, fn func() error, claimed bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	cand := o.taskByRef[taskID]
	if !o.claimableLocked(cand) {
		return nil, nil, false
	}
	st, fn = o.claimLocked(cand)
	return st, fn, true
}

// claimAnyForWaiter marks whichever submitted, eligible, not-yet-started
// task the scheduler reaches first as running for a waiting goroutine —
// claimForWaiter without a named target.
func (o *Output) claimAnyForWaiter() (st *taskState, fn func() error, claimed bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sched.cancelled {
		return nil, nil, false
	}
	if cand := o.nextEligibleLocked(); cand != nil {
		st, fn = o.claimLocked(cand)
		return st, fn, true
	}
	return nil, nil, false
}

// claimableLocked reports whether a waiting goroutine may run cand itself.
func (o *Output) claimableLocked(cand *taskState) bool {
	if o.sched.cancelled || cand == nil || !cand.awaitingStart() {
		return false
	}
	return o.eligibleLocked(cand)
}

func (st *taskState) closeDoneLocked() {
	if st.doneCh == nil {
		return
	}
	st.doneOnce.Do(func() { close(st.doneCh) })
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
func (t *TaskHandle) Wait() error {
	if t == nil || t.out == nil {
		return nil
	}
	var stack waiterStack
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
func (t *TaskHandle) waitChecked(stack *waiterStack, seen *inputSeals) error {
	t.out.sealAwaitedInputs(t.id, seen)
	t.out.runWaitedWork(t.id)
	if !t.waitSubmitted(stack) {
		return t.out.unreachableWaitOutcome(t.id)
	}
	return t.out.waitOutcome(t.id)
}

// unreachableWaitOutcome names the task a released waiter was awaiting, so
// the row the callback fails carries the cycle rather than a bare sentinel.
func (o *Output) unreachableWaitOutcome(taskID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil {
		return ErrWaitDeadlock
	}
	return fmt.Errorf("%w: %s", ErrWaitDeadlock, st.name)
}

// waitOutcome is the truth Wait owes its caller: the error the callback
// returned, or — when the callback never ran at all — the reason it did not.
// Answering with the zero value of "what the callback returned" is how a
// waiter came to resolve Done directly above the row admitting the work it
// awaited never started.
func (o *Output) waitOutcome(taskID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	switch {
	case st == nil:
		return nil
	case st.workErr != nil:
		return st.workErr
	case st.sched.phase == phaseDeclared && (st.state == NotStarted || st.neverDefined()):
		// Declared but never Defined: there is no work to have succeeded.
		return fmt.Errorf("%w: %s was never defined", ErrNotStarted, st.name)
	case st.state == NotStarted:
		return ErrNotStarted
	case st.state == Cancelled:
		return cancelledWaitOutcome(st.summary)
	case st.state == Failed || st.state == Blocked:
		// The row already failed but its callback has not returned yet (it
		// resolved itself via Failf/Blockf, which settles the row at once),
		// or it returned nil after stating its own failure. Either way the
		// work did not succeed, and Wait must not say it did.
		return failedWaitOutcome(st.summary)
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

// waitSubmitted parks until the task resolves, and reports whether it did.
// False means the scheduler proved the wait could never be satisfied and
// released the caller instead of letting it hang (see
// releaseUnsatisfiableWaits).
func (t *TaskHandle) waitSubmitted(stack *waiterStack) bool {
	if t == nil || t.out == nil {
		return true
	}
	o := t.out
	o.mu.Lock()
	st := o.taskByRef[t.id]
	if st == nil || st.neverDefined() {
		o.mu.Unlock()
		return true
	}
	// A terminal task closed its doneCh in the same critical section that
	// set its state, so there is nothing to park for.
	if core.IsTerminalTask(st.state) {
		o.mu.Unlock()
		return true
	}
	ch := st.doneCh
	o.mu.Unlock()
	if ch == nil {
		return true
	}
	ticket := o.beginWait(t.id, stack.callbackDepth())
	defer o.endWait(ticket)
	select {
	case <-ch:
		return true
	case <-ticket.abort:
		return false
	}
}
