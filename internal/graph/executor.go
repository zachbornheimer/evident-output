package graph

import (
	"fmt"
	"runtime"
)

// executor is the run's execution state: the callbacks in flight, the
// goroutines parked in a wait, and the flags that decide whether anything new
// may start. Guarded by the Graph's mutex. Which Tasks are eligible, and where
// each stands, is the scheduler's (see scheduler); this is what runs them.
type executor struct {
	// ceiling is the configured bound on executing callbacks; zero or less
	// means the number of processors, at least one.
	ceiling int
	// inflight counts pooled worker slots in use, bounded by the ceiling;
	// maxObserved is its high-water mark.
	inflight    int
	maxObserved int
	// executing counts task callbacks currently running, pooled and donated
	// alike: inflight counts only the pooled slots, so it cannot answer
	// "is any callback still moving?".
	executing int
	// waits holds one ticket per goroutine parked in a wait. Together with
	// executing it decides whether the run can still progress (see
	// resolveStall).
	waits map[*WaitTicket]struct{}
	// callbackGoroutines counts, per goroutine, the task callbacks it is
	// running right now. A parked goroutine that one of them started may be
	// what that callback is blocked on (see heldCallbacksLocked).
	callbackGoroutines map[GoroutineID]int
	// consumers is, per goroutine, the stack of Tasks and builder gates it
	// is running (see enterConsumer).
	consumers map[GoroutineID][]*Task
	// cancelled stops dispatching anything new: after an interrupt the queue
	// is abandoned, not drained.
	cancelled bool
}

// WithMaxConcurrency bounds how many callbacks execute at once; zero or less
// means the number of processors.
func WithMaxConcurrency(n int) Option { return func(g *Graph) { g.exec.ceiling = n } }

// claim is a Task whose callback was handed to a goroutine to run. pooled
// means the claim took a scheduler slot the run must get back.
type claim struct {
	task   *Task
	work   Work
	pooled bool
}

// start runs the caller's bookkeeping for a Task that began.
func (c *claim) start() {
	if c.work.Started != nil {
		c.work.Started()
	}
}

// Kick starts every Task the concurrency ceiling has room for, then ends a
// stall if the run can no longer move on its own (see resolveStall). A freed
// slot and a settled Task are the two events that can change either, and
// Kick is the choke point for both. The caller holds no lock Started takes.
func (g *Graph) Kick() {
	for {
		for {
			c := g.takeEligible()
			if c == nil {
				break
			}
			c.start()
			go g.runPooled(c)
		}
		if !g.resolveStall() {
			return
		}
	}
}

// takeEligible claims the earliest queued Task that may start now and takes
// a pooled slot for it, or returns nil.
func (g *Graph) takeEligible() *claim {
	g.lock()
	defer g.unlock()
	// After an interrupt the queue is abandoned, not drained: nothing new
	// starts, so the run stops at the ^C instead of running to completion
	// behind one cancelled row.
	if g.exec.cancelled || g.exec.inflight >= g.ceilingLocked() {
		return nil
	}
	cand := g.nextEligibleLocked()
	if cand == nil {
		return nil
	}
	g.takeSlotLocked()
	return g.claimLocked(cand, true)
}

// takeSlotLocked takes one scheduler slot; finishClaimed returns it.
func (g *Graph) takeSlotLocked() {
	g.exec.inflight++
	g.exec.maxObserved = max(g.exec.maxObserved, g.exec.inflight)
}

// claimLocked marks t's callback as started, for a pooled worker (which also
// took a slot) or for a waiter that runs it on its own goroutine.
func (g *Graph) claimLocked(t *Task, pooled bool) *claim {
	g.enterPhaseLocked(t, PhaseRunning)
	g.exec.executing++
	return &claim{task: t, work: t.sched.work, pooled: pooled}
}

func (g *Graph) ceilingLocked() int {
	if g.exec.ceiling > 0 {
		return g.exec.ceiling
	}
	return max(1, runtime.GOMAXPROCS(0))
}

// runPooled runs a Task claimed by Kick on a pooled worker.
func (g *Graph) runPooled(c *claim) {
	defer g.finishClaimed(c)
	g.execute(c)
}

// runClaimed runs a waiter's claim on the waiting goroutine, and reports
// whether there was one.
func (g *Graph) runClaimed(c *claim) bool {
	if c == nil {
		return false
	}
	c.start()
	defer g.finishClaimed(c)
	g.execute(c)
	return true
}

// finishClaimed is the deferred tail of every claimed callback: a panic
// fails the row, the slot (pooled only) and the executing count are
// returned, and the scheduler is kicked.
func (g *Graph) finishClaimed(c *claim) {
	if r := recover(); r != nil && c.work.Panicked != nil {
		c.work.Panicked(fmt.Sprintf("panic: %v", r))
	}
	g.lock()
	if c.pooled {
		g.exec.inflight--
	}
	g.exec.executing--
	g.unlock()
	g.WorkDone()
	g.Kick()
}

// execute runs one Task's callback and settles the Task from what it
// returned: the scheduler's sole settlement point for submitted work. Shared
// by the pooled worker and by a waiter that donates its own goroutine to work
// it would otherwise block on.
func (g *Graph) execute(c *claim) {
	defer g.enterConsumer(c.task)()
	if c.task.IsGate() {
		g.runGate(c)
		return
	}
	var err error
	if c.work.Run != nil {
		err = g.runTrackedCallback(c.work.Run)
	}
	g.recordWorkErr(c.task, err)
	// A callback that settled its own Task (a verb inside Run, or an
	// interrupt that cancelled the row) already stated one outcome. The
	// scheduler neither restates it nor calls it misuse: returning the same
	// error it already reported is the documented Fail-then-return-error
	// shape.
	if !g.settled(c.task) && c.work.Observed != nil {
		c.work.Observed(err)
	}
	if err != nil {
		g.FailSequenceFollowers(c.task)
	}
}

func (g *Graph) recordWorkErr(t *Task, err error) {
	g.lock()
	defer g.unlock()
	t.workErr = err
}

// settled reports whether t reached a terminal state.
func (g *Graph) settled(t *Task) bool {
	g.lock()
	defer g.unlock()
	return terminal(t)
}

// Drain runs the queue to empty. Once draining, a Task nobody Defined and an
// empty collection stop pending, so every parked Task is placed again before
// the drain starts. The caller holds no lock Started takes.
func (g *Graph) Drain() {
	g.BeginDrain()
	g.Kick()
	g.WaitForWork()
}

// Executing is how many task callbacks are running now.
func (g *Graph) Executing() int {
	g.lock()
	defer g.unlock()
	return g.exec.executing
}

// MaxObserved is the most pooled workers that ever ran at once.
func (g *Graph) MaxObserved() int {
	g.lock()
	defer g.unlock()
	return g.exec.maxObserved
}
