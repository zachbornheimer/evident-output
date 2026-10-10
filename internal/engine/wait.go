package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// waitTicket is one goroutine's registration while it is parked in
// TaskHandle.Wait: the task it awaits, how many task callbacks it is
// holding still on its own stack, the goroutine that started it, and the
// channel the scheduler closes to release it when it proves the wait can
// never be satisfied. released is the error that release hands the waiter.
type waitTicket struct {
	taskID   string
	depth    int
	self     graph.GoroutineID
	creator  graph.GoroutineID
	abort    chan struct{}
	released error
}

// beginWait registers this goroutine's park and re-tests the run: a newly
// parked waiter may be the last thing that could have moved it.
func (o *Output) beginWait(taskID string, depth int) *waitTicket {
	self, creator := graph.CurrentGoroutineLineage()
	ticket := &waitTicket{taskID: taskID, depth: depth, self: self, creator: creator, abort: make(chan struct{})}
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
// A waiting callback has stopped doing work, so the concurrency ceiling
// must never be the reason the task it waits on cannot start (P16).
// Donating only to the awaited task was not enough: at MaxConcurrency 1 the
// waiter's own slot can be the only thing keeping that task's unmet
// predecessor queued, so the wait ended only when the run drained and
// abandoned the whole chain. A plain caller holds no slot to lend: it runs
// work only in a free slot, and otherwise parks until the pool runs it (see
// takeWaiterSlotLocked).
//
// The loop terminates: every donation resolves one task, and a resolved task
// is never claimable again.
func (o *Output) runWaitedWork(taskID string, stack *waiterStack) {
	for {
		if o.runClaimed(o.claimForWaiter(taskID, stack)) {
			return
		}
		if !o.awaitedWorkIsStalled(taskID) {
			return
		}
		if !o.runClaimed(o.claimAnyForWaiter(stack)) {
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

// waiterClaim is one task a waiting goroutine claimed to run itself.
// pooled means the claim took a scheduler slot the run must get back.
type waiterClaim struct {
	st     *taskState
	fn     func() error
	pooled bool
}

// runClaimed executes a waiter's claim on the waiting goroutine, and
// reports whether there was one.
func (o *Output) runClaimed(c *waiterClaim) bool {
	if c == nil {
		return false
	}
	defer o.finishClaimed(c.st, c.pooled)
	o.executeWork(c.st, c.fn)
	return true
}

// claimForWaiter claims the named task for a waiting goroutine when it is
// submitted, eligible, and not yet started, and the waiter may run work
// now; nil otherwise.
func (o *Output) claimForWaiter(taskID string, stack *waiterStack) *waiterClaim {
	o.mu.Lock()
	defer o.mu.Unlock()
	cand := o.taskByRef[taskID]
	if !o.claimableLocked(cand) {
		return nil
	}
	return o.claimForWaiterLocked(cand, stack)
}

// claimAnyForWaiter is claimForWaiter for whichever eligible task the
// scheduler reaches first.
func (o *Output) claimAnyForWaiter(stack *waiterStack) *waiterClaim {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sched.cancelled {
		return nil
	}
	cand := o.nextEligibleLocked()
	if cand == nil {
		return nil
	}
	return o.claimForWaiterLocked(cand, stack)
}

// claimForWaiterLocked claims cand for a waiting goroutine.
func (o *Output) claimForWaiterLocked(cand *taskState, stack *waiterStack) *waiterClaim {
	pooled, ok := o.takeWaiterSlotLocked(stack)
	if !ok {
		return nil
	}
	st, fn := o.claimLocked(cand)
	return &waiterClaim{st: st, fn: fn, pooled: pooled}
}

// takeWaiterSlotLocked decides whether a waiting goroutine may run work
// now, so the number of executing callbacks never rises above the ceiling.
// A callback lends the slot it already holds (pooled is false). A plain
// caller holds none: it takes a free slot like a pooled worker would, and
// with none free it runs nothing — the awaited task included — and parks
// until the pool starts that task in the next slot to free up.
func (o *Output) takeWaiterSlotLocked(stack *waiterStack) (pooled, ok bool) {
	if stack.callbackDepth() > 0 {
		return false, true
	}
	if o.sched.inflight >= o.concurrencyCeilingLocked() {
		return false, false
	}
	o.takeSlotLocked()
	return true, true
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
	t.out.runWaitedWork(t.id, stack)
	if err := t.waitSubmitted(stack); err != nil {
		return err
	}
	return t.out.waitOutcome(t.id)
}

// unreachableWaitLocked names the task a released waiter was awaiting, so
// the row the callback fails carries the cycle rather than a bare sentinel.
// A waiter a live callback started is told the shape that stranded it.
func (o *Output) unreachableWaitLocked(ticket *waitTicket) error {
	st := o.taskByRef[ticket.taskID]
	if st == nil {
		return ErrWaitDeadlock
	}
	if o.startedByCallbackLocked(ticket) {
		return fmt.Errorf("%w: %s: waited from a goroutine a Task callback started while every slot was held; "+
			"if that callback blocks on this goroutine neither can move — Wait inside the callback, "+
			"or declare the work as Tasks under a Group (API-041)", ErrWaitDeadlock, st.name)
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
	case st.sched.phase == phaseDeclared && (st.rec.State() == NotStarted || st.neverDefined()):
		// Declared but never Defined: there is no work to have succeeded.
		return fmt.Errorf("%w: %s was never defined", ErrNotStarted, st.name)
	case st.rec.State() == NotStarted:
		return ErrNotStarted
	case st.rec.State() == Cancelled:
		return cancelledWaitOutcome(st.rec.Summary())
	case st.rec.State() == Failed || st.rec.State() == Blocked:
		// The row already failed but its callback has not returned yet (it
		// resolved itself via Fail/Block, which settles the row at once),
		// or it returned nil after stating its own failure. Either way the
		// work did not succeed, and Wait must not say it did.
		return failedWaitOutcome(st.rec.Summary())
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
// caller instead of letting it hang (see releaseWaitsLocked).
func (t *TaskHandle) waitSubmitted(stack *waiterStack) error {
	if t == nil || t.out == nil {
		return nil
	}
	o := t.out
	o.mu.Lock()
	st := o.taskByRef[t.id]
	if st == nil || st.neverDefined() {
		o.mu.Unlock()
		return nil
	}
	// A terminal task closed its doneCh in the same critical section that
	// set its state, so there is nothing to park for.
	if core.IsTerminalTask(st.rec.State()) {
		o.mu.Unlock()
		return nil
	}
	ch := st.doneCh
	o.mu.Unlock()
	if ch == nil {
		return nil
	}
	ticket := o.beginWait(t.id, stack.callbackDepth())
	defer o.endWait(ticket)
	select {
	case <-ch:
		return nil
	case <-ticket.abort:
		return ticket.released
	}
}
