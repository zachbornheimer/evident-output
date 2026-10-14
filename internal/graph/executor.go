package graph

import (
	"runtime"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// executor is the run's execution state: the callbacks in flight, the
// goroutines parked in a wait, and the flags that decide whether anything new
// may start. Guarded by the Graph's mutex. Which Tasks are eligible, and where
// each stands, is the scheduler's (see scheduler); this is what runs them.
type executor struct {
	// ceiling is the bound on executing callbacks: the configured one, or
	// the number of processors when the graph was built, at least one. It
	// is read once because asking the runtime takes its scheduler's lock.
	ceiling int
	// inflight counts pooled worker slots in use, bounded by the ceiling;
	// maxObserved is its high-water mark.
	inflight    int
	maxObserved int
	// executing counts task callbacks currently running: inflight counts the
	// pooled slots taken, so it cannot answer "is any callback still moving?".
	// It moves under the lock but is read without it: every declaration asks.
	executing atomic.Int64
	// cancelled stops dispatching anything new: after an interrupt the queue
	// is abandoned, not drained.
	cancelled bool
	// misuseNotes is the misuse found under the lock that no one has told
	// the MisuseSink yet (see noteMisuseLocked).
	misuseNotes []misuseNote
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
	// began is whether the Task started: set by start, before any goroutine
	// runs the claim.
	began bool
	// frame is the claim's work on the goroutine running it, once entered
	// (see enter).
	frame workFrame
	// returned is whether the callback ran to its end: false for one that
	// left through runtime.Goexit.
	returned bool
	// next links the claims one scheduling pass made, in claim order.
	next *claim
}

// start runs the caller's bookkeeping for a Task that began, and records
// whether it did.
func (c *claim) start() {
	c.began = c.work.Started == nil || c.work.Started()
}

// Kick starts every Task the concurrency ceiling has room for, then ends a
// stall if the run can no longer move on its own (see resolveStall). A freed
// slot and a settled Task are the two events that can change either, and
// Kick is the choke point for both. The caller holds no lock Started takes.
//
// Misuse the pass finds reaches the MisuseSink once scheduling is done and
// no lock is held, so a strict sink's panic unwinds into the caller.
func (g *Graph) Kick() { g.kick(g.tellMisuse) }

// kick is Kick with tell deciding where the misuse it found goes. Each
// scheduling pass is one critical section (see schedulingPass); the claims
// it made start once the lock is free, since Started takes the run's lock,
// which comes first in the lock order.
func (g *Graph) kick(tell func([]misuseNote)) { g.kickAfter(nil, tell, nil) }

// kickAfter is kick whose first scheduling pass runs release first, in the
// same critical section: a finishing worker hands its slot back and claims
// the next Task in one trip through the graph's lock. A finishing worker
// (nil for any other caller) runs the first Task it claims itself.
func (g *Graph) kickAfter(release func(), tell func([]misuseNote), self *worker) {
	var found []misuseNote
	defer func() { tell(found) }()
	for {
		claims, moved, notes := g.schedulingPass(release)
		release = nil
		found = append(found, notes...)
		for c := claims; c != nil; c = c.next {
			c.start()
			g.place(c, self)
		}
		if !moved {
			return
		}
	}
}

// schedulingPass claims every Task that may start now, in queue order, then
// ends a stall if the run can no longer move on its own (see resolveStall).
// It reports the claims, linked in order, whether the stall step changed
// anything, and the misuse it found.
func (g *Graph) schedulingPass(release func()) (claims *claim, moved bool, notes []misuseNote) {
	g.lock()
	defer g.unlock()
	if release != nil {
		release()
	}
	last := &claims
	for c := g.takeEligibleLocked(); c != nil; c = g.takeEligibleLocked() {
		*last = c
		last = &c.next
	}
	moved, notes = g.resolveStallLocked()
	return claims, moved, notes
}

// takeEligibleLocked claims the earliest queued Task that may start now and
// takes a pooled slot for it, or returns nil.
func (g *Graph) takeEligibleLocked() *claim {
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
// took a slot) or for a waiter that runs it on its own goroutine. The Task's
// row is Running in the same step as the claim, so no moment exists in which
// the scheduler runs a Task whose row still reads Pending: an interrupt
// always finds a claimed Task Running.
func (g *Graph) claimLocked(t *Task, pooled bool) *claim {
	g.enterPhaseLocked(t, PhaseRunning)
	if t.gateFor == nil && t.Rec.State() == record.Pending {
		t.Rec.Transition(record.Running)
	}
	g.exec.executing.Add(1)
	return &claim{task: t, work: t.sched.work, pooled: pooled}
}

func (g *Graph) ceilingLocked() int { return g.exec.ceiling }

// resolveCeiling fixes an unconfigured ceiling (zero or less) at the number
// of processors, at least one.
func (e *executor) resolveCeiling() {
	if e.ceiling <= 0 {
		e.ceiling = max(1, runtime.GOMAXPROCS(0))
	}
}

// runPooled runs a Task claimed by Kick on worker w.
func (g *Graph) runPooled(c *claim, w *worker) {
	defer g.finishClaimed(c, w)
	g.execute(c, w.id)
	c.returned = true
}

// finishClaimed is the deferred tail of every claimed callback: a panic
// fails the row, the goroutine leaves the claim's work frame, and then, in
// the critical section of the scheduling pass that follows, the slot
// (pooled only) and the executing count are returned. A pooled worker has no
// caller to take a strict panic, so what its pass finds is told without one.
// Last, it settles whether the worker may run another claim (see worker) and
// gives away the claim its pass kept for itself when it may not.
//
// The work is done only after that pass: a drain that wakes on it finds the
// slot already returned and the next Task already claimed.
func (g *Graph) finishClaimed(c *claim, w *worker) {
	recovered := recover()
	if recovered != nil {
		g.recordPanic(c, recovered)
	} else if !c.returned {
		g.recordGoexit(c)
	}
	defer g.WorkDone()
	g.frames.leave(c.frame)
	g.kickAfter(func() { g.releaseLocked(c) }, g.tellMisuseWithoutCaller, w)
	w.reusable = (recovered != nil || c.returned) && !w.spawned && workerSpawns.quiet(w.opened)
	g.rehome(w)
}

// releaseLocked hands back what c held.
func (g *Graph) releaseLocked(c *claim) {
	if c.pooled {
		g.exec.inflight--
	}
	g.exec.executing.Add(-1)
}

// recordPanic states a panic that escaped c's callback. The row fails with the
// panic text.
func (g *Graph) recordPanic(c *claim, recovered any) {
	if c.work.Panicked != nil {
		c.work.Panicked(panicError(recovered).Error())
	}
}

// goexitSummary is the ending of a callback that left through
// runtime.Goexit (a direct call, or a library such as testing.T.FailNow).
const goexitSummary = "callback exited without returning (runtime.Goexit)"

// recordGoexit states the ending of a callback that left through
// runtime.Goexit. A Task the callback already settled keeps its outcome.
func (g *Graph) recordGoexit(c *claim) {
	if c.work.Exited != nil && !terminal(c.task) {
		c.work.Exited(goexitSummary)
	}
}

// execute runs one Task's callback on a pooled worker and settles the Task
// from what it returned: the scheduler's sole settlement point for submitted
// work. A pooled worker has started no goroutine before (see worker), so every
// goroutine it starts is the callback's. finishClaimed leaves the frame execute
// enters.
func (g *Graph) execute(c *claim, id GoroutineID) {
	if !c.began {
		return
	}
	c.frame = g.frames.enter(c.task, id)
	if c.task.IsGate() {
		g.runGate(c)
		return
	}
	var err error
	if c.work.Run != nil {
		err = g.failedByRefusedRead(c.task, runCallback(c.work.Run))
	}
	// A callback that settled its own Task (a verb inside Run, or an
	// interrupt that cancelled the row) already stated one outcome. The
	// scheduler neither restates it nor calls it misuse: returning the same
	// error it already reported is the documented Fail-then-return-error
	// shape.
	if !terminal(c.task) && c.work.Observed != nil {
		c.work.Observed(err)
	}
	if err != nil {
		g.FailSequenceFollowers(c.task)
	}
}

// Drain runs the queue to empty. Once draining, a Task nobody Defined and an
// empty collection stop pending, so every parked Task is placed again before
// the drain starts. The caller holds no lock Started takes.
func (g *Graph) Drain() {
	g.BeginDrain()
	g.Kick()
	g.WaitForWork()
	g.workers.dismiss()
}

// Executing is how many task callbacks are running now.
func (g *Graph) Executing() int { return int(g.exec.executing.Load()) }

// MaxObserved is the most pooled workers that ever ran at once.
func (g *Graph) MaxObserved() int {
	g.lock()
	defer g.unlock()
	return g.exec.maxObserved
}
