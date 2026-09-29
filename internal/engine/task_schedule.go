package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine/schedule"
)

// taskSchedule is one Task's scheduling state: its standing, the work
// Define submitted, what it must wait for, and who is waiting for it.
// Guarded by Output.mu.
type taskSchedule struct {
	standing schedule.Standing
	work     func() error
	// preds are the Task's predecessors: its After arguments and, inside
	// a Sequence, the step declared just before its own (see
	// nextStepPreds).
	preds []predecessor
	// dependents are the Tasks parked until this one resolves.
	dependents []*taskState
}

// awaitingStart reports whether st is submitted work nobody has started or
// resolved.
func (st *taskState) awaitingStart() bool {
	return st.sched.standing.AwaitingStart() && !core.IsTerminalTask(st.state.Current())
}

// neverDefined reports whether st is still waiting on its caller: declared,
// never Defined, and not resolved by a verb either.
func (st *taskState) neverDefined() bool {
	return st.sched.standing.Phase() == schedule.Declared && !core.IsTerminalTask(st.state.Current())
}

// queued reports whether st is still a live entry in the scheduler queue
// (was inline in schedQueue.head).
func (st *taskState) queued() bool {
	return st.sched.standing.Phase() == schedule.Queued && st.awaitingStart()
}

// Declaration is st's declaration order, for schedule.Member.
func (st *taskState) Declaration() int { return st.declaration }

// Outcome is st's raw state outcome, for schedule.Member.
func (st *taskState) Outcome() schedule.Outcome { return schedule.OutcomeOf(st.state.Current()) }
