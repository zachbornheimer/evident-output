package engine

import (
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Define lives in verify.go, alongside its Verify-aware execution wiring
// (runDefine) — both are one concern (§7, §9.1). The graph owns the executor:
// when a callback starts, how many run at once, and what a stall or an
// interrupt does to the Tasks behind it (see graph.Graph.Kick).

func (t *TaskHandle) submitWork(run func() error) {
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
	// §48: a predecessor that already failed settles this Task NotStarted
	// right here, under the same lock, and so does an interrupt.
	switch o.graph.Submit(st.node, o.workOf(st, run)) {
	case graph.AlreadySettled:
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		o.mu.Unlock()
		return
	case graph.AlreadySubmitted:
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	o.followRecordLocked()
	o.mu.Unlock()
	o.graph.Kick()
}

// workOf is what the graph runs for st: the callback, and the engine's own
// steps around it.
func (o *Output) workOf(st *taskState, run func() error) graph.Work {
	return graph.Work{
		Started:  func() { o.taskStarted(st) },
		Run:      run,
		Observed: func(err error) { o.resolveObserved(st, err) },
		Panicked: st.handle.failScheduled,
	}
}

// taskStarted is the render work a claimed Task owes: the eligibility event,
// the move to Running, the snapshot version and a forced paint, within the
// live render budget, since a start is the spinner FP-005 requires before the
// check.
func (o *Output) taskStarted(st *taskState) {
	if st.isGate() {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventTaskEligible, st.id, nil)
	if st.node.Rec.State() == Pending {
		o.promoteRunningLocked(st)
	}
	o.bumpLocked()
	o.signalLiveLocked(true)
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

// drainScheduler runs the queue to empty.
func (o *Output) drainScheduler() { o.graph.Drain() }
