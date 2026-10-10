package engine

import (
	"fmt"
	"runtime"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Define lives in verify.go, alongside its Verify-aware execution wiring
// (runDefine) — both are one concern (§7, §9.1).

func (t *TaskHandle) submitWork(fn func() error) {
	if t == nil || t.out == nil {
		return
	}
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	if st == nil {
		o.mu.Unlock()
		return
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		o.mu.Unlock()
		return
	}
	if core.IsTerminalTask(st.rec.State()) {
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		o.mu.Unlock()
		return
	}
	if st.node.Submitted() {
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	o.graph.Enqueue(st.node, fn)
	if o.sched.cancelled {
		// Nothing starts after an interrupt, so work submitted after it is
		// work the interrupt took away (see abandonQueuedWork).
		o.markNotStartedLocked(st)
		o.mu.Unlock()
		return
	}
	// §48: a predecessor that already failed settles this Task NotStarted
	// right here, under the same lock.
	o.graph.Place(st.node)
	o.mu.Unlock()
	o.kick()
}

// kick starts every Task the concurrency ceiling has room for, then ends a
// stall if the run can no longer move on its own (see resolveStall). A
// freed slot and a settled Task are the two events that can change either,
// and kick is the choke point for both.
func (o *Output) kick() {
	for {
		for {
			st, fn := o.takeEligible()
			if st == nil {
				break
			}
			go o.runWork(st, fn)
		}
		if !o.resolveStall() {
			return
		}
	}
}

// nextEligibleLocked returns the earliest queued Task that may start now,
// or nil.
func (o *Output) nextEligibleLocked() *taskState {
	node := o.graph.NextEligible()
	if node == nil {
		return nil
	}
	return o.taskStates[node.ID]
}

func (o *Output) takeEligible() (st *taskState, fn func() error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sched.cancelled {
		// After an interrupt the queue is abandoned, not drained: nothing
		// new starts, so the run stops at the ^C instead of running to
		// completion behind one cancelled row.
		return nil, nil
	}
	if o.sched.inflight >= o.concurrencyCeilingLocked() {
		return nil, nil
	}
	cand := o.nextEligibleLocked()
	if cand == nil {
		return nil, nil
	}
	if !cand.isGate() {
		o.emitWireEventLocked(wire.EventTaskEligible, cand.id, nil)
	}
	o.takeSlotLocked()
	return o.claimLocked(cand)
}

// takeSlotLocked takes one scheduler slot; finishClaimed returns it.
func (o *Output) takeSlotLocked() {
	o.sched.inflight++
	o.sched.maxObserved = max(o.sched.maxObserved, o.sched.inflight)
}

// claimLocked marks cand's callback as started — for a pooled worker
// (takeEligible, which also takes a slot) or for a waiter that runs it on
// its own goroutine (see runWaitedWork).
func (o *Output) claimLocked(cand *taskState) (st *taskState, fn func() error) {
	o.graph.Claim(cand.node)
	o.sched.executing++
	if cand.isGate() {
		return cand, cand.node.Work()
	}
	if cand.rec.State() == Pending {
		o.promoteRunningLocked(cand)
	}
	o.bumpLocked()
	// Forced, within the live render budget: a start is the spinner FP-005
	// requires before the check.
	o.signalLiveLocked(true)
	return cand, cand.node.Work()
}

func (o *Output) concurrencyCeilingLocked() int {
	if o.cfg.maxConcurrency > 0 {
		return o.cfg.maxConcurrency
	}
	return max(1, runtime.GOMAXPROCS(0))
}

// runWork runs a Task claimed by takeEligible on a pooled worker.
func (o *Output) runWork(st *taskState, fn func() error) {
	defer o.finishClaimed(st, true)
	o.executeWork(st, fn)
}

// finishClaimed is the deferred tail of every claimed callback: a panic
// fails the row, the slot (pooled only) and the executing count are
// returned, and the scheduler is kicked.
func (o *Output) finishClaimed(st *taskState, pooled bool) {
	if r := recover(); r != nil {
		st.handle.failScheduled(fmt.Sprintf("panic: %v", r))
	}
	o.mu.Lock()
	if pooled {
		o.sched.inflight--
	}
	o.sched.executing--
	o.mu.Unlock()
	o.graph.WorkDone()
	o.kick()
}

// executeWork runs one task's callback and resolves the task from what it
// returned — the scheduler's sole resolution point (see TaskHandle.finish).
// Shared by the pooled worker (runWork) and by a waiter that donates its own
// goroutine to work it would otherwise block on (TaskHandle.Wait).
func (o *Output) executeWork(st *taskState, fn func() error) {
	defer o.enterConsumer(st)()
	if st.isGate() {
		o.runGate(st)
		return
	}
	var err error
	if fn != nil {
		err = o.runTrackedCallback(fn)
	}
	o.recordWorkOutcome(st, err)
	// A callback that resolved its own task (Fail/Block inside fn, or
	// an interrupt that cancelled the row) already stated one outcome. The
	// scheduler neither restates it nor calls it misuse (P13) — returning
	// the same error it already reported is the documented Fail-then-return-error shape.
	if !o.taskIsTerminal(st) {
		o.resolveObserved(st, err)
	}
	if err != nil {
		o.failSequenceFollowers(st)
	}
}

// resolveObserved commits the task's outcome from what the callback actually
// returned, ratifying or discarding the caller's proposal (see
// TaskHandle.finish). A ratified proposal supplies the row's summary — the
// caller's own words, now backed by an observation. A returned error
// replaces the proposal: it was never a resolution, so it earns no
// "resolve each task once" misuse line (E-113).
func (o *Output) resolveObserved(st *taskState, err error) {
	proposal := st.node.TakeProposal()
	if err != nil {
		st.handle.failScheduled(err.Error())
		return
	}
	if proposal != nil {
		st.handle.resolveScheduled(proposal.State, proposal.Summary, proposal.Problems)
		return
	}
	st.handle.doneScheduled()
}

// recordWorkOutcome stores the callback's error on the task so a waiter
// (TaskHandle.Wait) can return the same value the callback returned.
func (o *Output) recordWorkOutcome(st *taskState, err error) {
	if st == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	st.workErr = err
}

func (o *Output) taskIsTerminal(st *taskState) bool {
	if st == nil {
		return true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return core.IsTerminalTask(st.rec.State())
}

// runTrackedCallback runs fn while the scheduler knows which goroutine is
// running it, so a goroutine fn starts and then parks in Wait can be traced
// back to the callback it may be holding still.
func (o *Output) runTrackedCallback(fn func() error) error {
	g := graph.CurrentGoroutine()
	o.mu.Lock()
	if o.sched.callbackGoroutines == nil {
		o.sched.callbackGoroutines = make(map[graph.GoroutineID]int)
	}
	o.sched.callbackGoroutines[g]++
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		if o.sched.callbackGoroutines[g]--; o.sched.callbackGoroutines[g] <= 0 {
			delete(o.sched.callbackGoroutines, g)
		}
		o.mu.Unlock()
	}()
	return runCallback(fn)
}

// runCallback is the single frame every task callback runs beneath, so a
// goroutine parked in Wait can count how many callbacks it is holding
// still. That count is the one thing separating "a callback is stuck
// waiting" from "a plain caller is waiting while callbacks run".
func runCallback(fn func() error) error {
	callbackFrames.Note()
	return fn()
}

// callbackFrames marks runCallback (see frameMarker).
var callbackFrames graph.FrameMarker

// callbackDepth counts the task callbacks the calling goroutine is
// currently inside: zero for a plain caller, one for a callback, more when
// a waiter donated its goroutine to nested work before parking.
func callbackDepth() int { return readStackMarks().callbacks }

// drainScheduler runs the queue to empty. Once draining, a Task nobody
// Defined and an empty collection stop pending, so every parked Task is
// placed again before the drain starts.
func (o *Output) drainScheduler() {
	o.mu.Lock()
	o.graph.BeginDrain()
	o.mu.Unlock()
	o.kick()
	o.graph.WaitForWork()
}

// markNotStartedLocked settles st NotStarted: work that will now never run.
func (o *Output) markNotStartedLocked(st *taskState) {
	o.graph.MarkNotStarted(st.node)
	o.followRecordLocked()
}

// failSequenceFollowers settles NotStarted every step declared after the
// one failed belongs to, in each Sequence it sits under, that has not
// started. It runs under o.mu so a failure settles its followers in the same
// critical section that makes it terminal: nobody who sees the failure sees
// a follower pending.
func (o *Output) failSequenceFollowers(failed *taskState) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.graph.FailSequenceFollowers(failed.node)
	o.followRecordLocked()
}
