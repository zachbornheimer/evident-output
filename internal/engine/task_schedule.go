package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine/schedule"
)

// schedPhase is where a Task stands with the scheduler. The phases are
// closed: every scheduling predicate reads one of them instead of
// re-deriving it from a conjunction of flags.
type schedPhase uint8

const (
	// phaseDeclared: the Task exists but was never Defined. Its verb is
	// still to come from the caller (or it resolved by a caller verb).
	phaseDeclared schedPhase = iota
	// phaseQueued: Defined, every predecessor succeeded, waiting in the
	// scheduler queue for a slot.
	phaseQueued
	// phaseParked: Defined, and waiting off the queue on the one
	// predecessor it is blocked by (see parkLocked).
	phaseParked
	// phaseRunning: its callback was claimed by a worker or a waiter.
	phaseRunning
	// phaseAbandoned: Defined, but settled NotStarted without ever running.
	phaseAbandoned
)

// taskSchedule is one Task's scheduling state: its phase, the work Define
// submitted, what it must wait for, and who is waiting for it. Guarded by
// Output.mu.
type taskSchedule struct {
	phase schedPhase
	work  func() error
	// preds are the Task's predecessors: its After arguments and, inside
	// a Sequence, the step declared just before its own (see
	// nextStepPreds).
	preds []predecessor
	// dependents are the Tasks parked until this one resolves.
	dependents []*taskState
}

// submitted reports whether Define submitted this Task's work and the
// scheduler has not abandoned it: its configuration is frozen and only the
// callback's return decides its outcome.
func (s *taskSchedule) submitted() bool {
	return s.phase == phaseQueued || s.phase == phaseParked || s.phase == phaseRunning
}

// awaitingStart reports whether the work was submitted and nobody has
// started it yet. The caller still checks the Task is not terminal.
func (s *taskSchedule) awaitingStart() bool {
	return s.phase == phaseQueued || s.phase == phaseParked
}

// awaitingStart reports whether st is submitted work nobody has started or
// resolved.
func (st *taskState) awaitingStart() bool {
	return st.sched.awaitingStart() && !core.IsTerminalTask(st.state.Current())
}

// neverDefined reports whether st is still waiting on its caller: declared,
// never Defined, and not resolved by a verb either.
func (st *taskState) neverDefined() bool {
	return st.sched.phase == phaseDeclared && !core.IsTerminalTask(st.state.Current())
}

// Declaration is st's declaration order, for schedule.Member.
func (st *taskState) Declaration() int { return st.declaration }

// Outcome is st's raw state outcome, for schedule.Member.
func (st *taskState) Outcome() schedule.Outcome { return schedule.OutcomeOf(st.state.Current()) }
