package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// resolveStall ends a stall: nothing in the run can move on its own, yet a
// Wait is parked or Finish is draining. Left alone, either would hang the
// run forever, so each step below turns what is stuck into a stated
// outcome. It reports whether it changed anything, so kick schedules
// again before trying the next step.
//
// The steps run in the order that leaves every row most truthful:
//
//  1. what a parked Wait still waits for and nothing will now supply — a
//     Task nobody Defined, an empty collection — is sealed;
//  2. a Task in an After cycle settles Blocked, naming the cycle;
//  3. a parked Wait is released with ErrWaitDeadlock;
//  4. once draining, any Task still parked settles NotStarted.
func (o *Output) resolveStall() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.sched.waits) == 0 && !o.sched.draining {
		return false
	}
	if o.progressPossibleLocked() {
		return false
	}
	if !o.sched.draining && o.sealWaitedInputsLocked() {
		return true
	}
	if o.blockCyclesLocked() {
		return true
	}
	if len(o.sched.waits) > 0 {
		o.releaseWaitsLocked()
		return true
	}
	return o.sched.draining && o.abandonStrandedLocked()
}

// releaseWaitsLocked ends the parked waits nothing left in the run can
// satisfy. Each waiter is told the truth — ErrWaitDeadlock naming the task
// it awaited — and its callback returns that error, putting the cycle on
// its own row.
//
// A waiter a live callback started is released first, and alone: the stall
// only assumed that callback is blocked on it, and once it returns the
// callback may finish and satisfy every other wait.
func (o *Output) releaseWaitsLocked() {
	release := o.spawnedWaitsLocked()
	if len(release) == 0 {
		for ticket := range o.sched.waits {
			release = append(release, ticket)
		}
	}
	for _, ticket := range release {
		ticket.released = o.unreachableWaitLocked(ticket)
		if st := o.taskByRef[ticket.taskID]; st != nil {
			o.recordMisuseFor(st.name, ErrWaitDeadlock)
		}
	}
	for _, ticket := range release {
		delete(o.sched.waits, ticket)
		close(ticket.abort)
	}
}

// spawnedWaitsLocked lists the parked waiters a live callback started.
func (o *Output) spawnedWaitsLocked() []*waitTicket {
	var spawned []*waitTicket
	for ticket := range o.sched.waits {
		if o.startedByCallbackLocked(ticket) {
			spawned = append(spawned, ticket)
		}
	}
	return spawned
}

// abandonStrandedLocked settles NotStarted every Task still parked once the
// drain can move no further: what it waits for will never resolve (a
// collection whose member nobody Defined). It reports whether any was.
func (o *Output) abandonStrandedLocked() bool {
	if o.sched.parked == 0 {
		return false
	}
	for _, st := range o.tasks {
		if st.sched.phase == phaseParked && !core.IsTerminalTask(st.rec.State()) {
			o.markNotStartedLocked(st)
		}
	}
	for _, gate := range o.sched.gates {
		if gate.sched.phase == phaseParked && !core.IsTerminalTask(gate.rec.State()) {
			o.markNotStartedLocked(gate)
		}
	}
	return true
}

// progressPossibleLocked reports whether anything could still move the run.
func (o *Output) progressPossibleLocked() bool {
	return o.sched.executing > o.heldCallbacksLocked() ||
		o.anyStartableLocked() ||
		o.anyAwaitedTaskResolvedLocked()
}

// heldCallbacksLocked counts the callbacks a parked waiter may be holding
// still: the ones on its own stack, and those of a callback goroutine that
// started it — the errgroup shape, where that callback blocks on the
// goroutine it started. More callbacks executing than held means one of
// them is still doing work.
func (o *Output) heldCallbacksLocked() int {
	held := 0
	parked := make(map[graph.GoroutineID]struct{}, len(o.sched.waits))
	for ticket := range o.sched.waits {
		held += ticket.depth
		parked[ticket.self] = struct{}{}
	}
	creators := make(map[graph.GoroutineID]struct{})
	for ticket := range o.sched.waits {
		if _, counted := parked[ticket.creator]; !counted && o.startedByCallbackLocked(ticket) {
			creators[ticket.creator] = struct{}{}
		}
	}
	for creator := range creators {
		held += o.sched.callbackGoroutines[creator]
	}
	return held
}

// startedByCallbackLocked reports whether ticket's goroutine was started by
// a goroutine that is running a task callback right now. A parked waiter's
// own callbacks are already counted by its depth, so only a plain waiter
// can be one.
func (o *Output) startedByCallbackLocked(ticket *waitTicket) bool {
	return ticket.depth == 0 && ticket.creator != 0 && o.sched.callbackGoroutines[ticket.creator] > 0
}

// anyStartableLocked reports whether the pool could start queued work now:
// something is eligible and a slot is free. Eligible work behind a full
// pool moves only when an executing callback finishes, which the executing
// count already answers.
func (o *Output) anyStartableLocked() bool {
	return !o.sched.cancelled &&
		o.sched.inflight < o.concurrencyCeilingLocked() &&
		o.nextEligibleLocked() != nil
}

// anyAwaitedTaskResolvedLocked reports whether some parked waiter's task is
// already terminal — it is about to wake on its own doneCh.
func (o *Output) anyAwaitedTaskResolvedLocked() bool {
	for ticket := range o.sched.waits {
		if st := o.taskByRef[ticket.taskID]; st != nil && core.IsTerminalTask(st.rec.State()) {
			return true
		}
	}
	return false
}
