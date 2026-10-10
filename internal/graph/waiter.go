package graph

import (
	"errors"
	"fmt"
)

// ErrWaitDeadlock is why a wait was released instead of satisfied: nothing
// left in the run can ever reach the Task it awaits.
var ErrWaitDeadlock = errors.New("evo: awaited task can never be reached")

// WaitTicket is one goroutine's registration while it is parked waiting for a
// Task: the Task it awaits, how many task callbacks it is holding still on its
// own stack, the goroutine that started it, and the channel the scheduler
// closes to release it when it proves the wait can never be satisfied.
type WaitTicket struct {
	task    *Task
	depth   int
	self    GoroutineID
	creator GoroutineID
	// owner is the Task or builder gate whose callback is waiting here, or
	// whose callback started the waiting goroutine; nil for a plain caller.
	owner    *Task
	abort    chan struct{}
	released error
}

// Aborted is closed when the scheduler releases the wait unsatisfied.
func (w *WaitTicket) Aborted() <-chan struct{} { return w.abort }

// Released is why the wait was released, valid once Aborted is closed.
func (w *WaitTicket) Released() error { return w.released }

// BeginWait registers the calling goroutine's park on t, which holds depth
// task callbacks still on its stack. The caller defers EndWait, then Kicks: a
// newly parked waiter may be the last thing that could have moved the run, and
// the Kick may unwind with a strict MisuseSink's panic, which must not leave
// the ticket behind.
func (g *Graph) BeginWait(t *Task, depth int) *WaitTicket {
	self, creator := CurrentGoroutineLineage()
	w := &WaitTicket{task: t, depth: depth, self: self, creator: creator, abort: make(chan struct{})}
	g.lock()
	w.owner = g.consumerThroughLocked(self, creator)
	if g.exec.waits == nil {
		g.exec.waits = make(map[*WaitTicket]struct{})
	}
	g.exec.waits[w] = struct{}{}
	g.unlock()
	return w
}

// EndWait ends the registration BeginWait made.
func (g *Graph) EndWait(w *WaitTicket) {
	g.lock()
	defer g.unlock()
	delete(g.exec.waits, w)
}

// Waits is how many goroutines are parked in a wait.
func (g *Graph) Waits() int {
	g.lock()
	defer g.unlock()
	return len(g.exec.waits)
}

// RunWaited executes, on the waiting caller's own goroutine, the work
// standing between it and t: t itself when the scheduler has not started it,
// and otherwise whatever the scheduler is holding back that could make it
// eligible.
//
// A waiting callback has stopped doing work, so the concurrency ceiling must
// never be the reason the Task it waits on cannot start. Donating only to the
// awaited Task was not enough: at a ceiling of one the waiter's own slot can
// be the only thing keeping that Task's unmet predecessor queued, so the wait
// ended only when the run drained and abandoned the whole chain. A plain
// caller holds no slot to lend: it runs work only in a free slot, and
// otherwise parks until the pool runs it (see takeWaiterSlotLocked).
//
// The loop terminates: every donation settles one Task, and a settled Task is
// never claimable again. The caller holds no lock Started takes.
func (g *Graph) RunWaited(t *Task, stack *WaiterStack) {
	for {
		if g.runClaimed(g.claimForWaiter(t, stack)) {
			return
		}
		if !g.awaitedWorkIsStalled(t) {
			return
		}
		if !g.runClaimed(g.claimAnyForWaiter(stack)) {
			return
		}
	}
}

// awaitedWorkIsStalled reports whether t is submitted work that no goroutine
// is executing: the only case where the waiter going to sleep is itself what
// prevents progress. A Task another goroutine is already running, or one
// already settled, needs no donation.
func (g *Graph) awaitedWorkIsStalled(t *Task) bool {
	g.lock()
	defer g.unlock()
	return t.awaitingStartLocked()
}

// claimForWaiter claims t for a waiting goroutine when it is submitted,
// eligible, and not yet started, and the waiter may run work now; nil
// otherwise.
func (g *Graph) claimForWaiter(t *Task, stack *WaiterStack) *claim {
	g.lock()
	defer g.unlock()
	if g.exec.cancelled || !t.awaitingStartLocked() || !g.eligibleLocked(t) {
		return nil
	}
	return g.claimForWaiterLocked(t, stack)
}

// claimAnyForWaiter is claimForWaiter for whichever eligible Task the
// scheduler reaches first.
func (g *Graph) claimAnyForWaiter(stack *WaiterStack) *claim {
	g.lock()
	defer g.unlock()
	if g.exec.cancelled {
		return nil
	}
	cand := g.nextEligibleLocked()
	if cand == nil {
		return nil
	}
	return g.claimForWaiterLocked(cand, stack)
}

func (g *Graph) claimForWaiterLocked(cand *Task, stack *WaiterStack) *claim {
	pooled, ok := g.takeWaiterSlotLocked(stack)
	if !ok {
		return nil
	}
	return g.claimLocked(cand, pooled)
}

// takeWaiterSlotLocked decides whether a waiting goroutine may run work now,
// so the number of executing callbacks never rises above the ceiling. A
// callback lends the slot it already holds (pooled is false). A plain caller
// holds none: it takes a free slot like a pooled worker would, and with none
// free it runs nothing, the awaited Task included, and parks until the pool
// starts that Task in the next slot to free up.
func (g *Graph) takeWaiterSlotLocked(stack *WaiterStack) (pooled, ok bool) {
	if stack.CallbackDepth() > 0 {
		return false, true
	}
	if g.exec.inflight >= g.ceilingLocked() {
		return false, false
	}
	g.takeSlotLocked()
	return true, true
}

// unreachableWait names the Task a released waiter was awaiting, so the row
// the callback fails carries the cycle rather than a bare sentinel. A waiter
// a live callback started is told the shape that stranded it.
func (g *Graph) unreachableWait(w *WaitTicket) error {
	if g.startedByCallbackLocked(w) {
		return fmt.Errorf("%w: %s: waited from a goroutine a Task callback started while every slot was held; "+
			"if that callback blocks on this goroutine neither can move — Wait inside the callback, "+
			"or declare the work as Tasks under a Group (API-041)", ErrWaitDeadlock, w.task.Name)
	}
	return fmt.Errorf("%w: %s", ErrWaitDeadlock, w.task.Name)
}
