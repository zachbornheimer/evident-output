package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
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

// releaseWaitsLocked ends every parked wait: nothing left in the run can
// satisfy it. Each waiter is told the truth — ErrWaitDeadlock naming the
// task it awaited — and its callback returns that error, putting the cycle
// on its own row.
func (o *Output) releaseWaitsLocked() {
	for ticket := range o.sched.waits {
		delete(o.sched.waits, ticket)
		if st := o.taskByRef[ticket.taskID]; st != nil {
			o.recordMisuseFor(st.name, ErrWaitDeadlock)
		}
		close(ticket.abort)
	}
}

// abandonStrandedLocked settles NotStarted every Task still parked once the
// drain can move no further: what it waits for will never resolve (a
// collection whose member nobody Defined). It reports whether any was.
func (o *Output) abandonStrandedLocked() bool {
	if o.sched.parked == 0 {
		return false
	}
	for _, st := range o.tasks {
		if st.sched.phase == phaseParked && !core.IsTerminalTask(st.state) {
			o.markNotStartedLocked(st)
		}
	}
	return true
}

// progressPossibleLocked reports whether anything could still move the run.
func (o *Output) progressPossibleLocked() bool {
	return o.sched.executing > o.parkedCallbacksLocked() ||
		o.anyClaimableLocked() ||
		o.anyAwaitedTaskResolvedLocked()
}

// parkedCallbacksLocked counts the callbacks currently held still by a
// parked waiter. More callbacks executing than parked means one of them is
// still doing work.
func (o *Output) parkedCallbacksLocked() int {
	parked := 0
	for ticket := range o.sched.waits {
		parked += ticket.depth
	}
	return parked
}

func (o *Output) anyClaimableLocked() bool {
	return !o.sched.cancelled && o.nextEligibleLocked() != nil
}

// anyAwaitedTaskResolvedLocked reports whether some parked waiter's task is
// already terminal — it is about to wake on its own doneCh.
func (o *Output) anyAwaitedTaskResolvedLocked() bool {
	for ticket := range o.sched.waits {
		if st := o.taskByRef[ticket.taskID]; st != nil && core.IsTerminalTask(st.state) {
			return true
		}
	}
	return false
}
