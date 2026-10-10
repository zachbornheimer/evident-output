package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// Phase is where a Task stands with the scheduler. The phases are closed:
// every scheduling predicate reads one of them instead of re-deriving it
// from a conjunction of flags.
type Phase uint8

const (
	// PhaseDeclared: the Task exists but was never Defined. Its verb is
	// still to come from the caller (or it resolved by a caller verb).
	PhaseDeclared Phase = iota
	// PhaseQueued: Defined, every predecessor succeeded, waiting in the
	// scheduler queue for a slot.
	PhaseQueued
	// PhaseParked: Defined, and waiting off the queue on the one
	// predecessor it is blocked by.
	PhaseParked
	// PhaseRunning: its callback was claimed by a worker or a waiter.
	PhaseRunning
	// PhaseAbandoned: Defined, but settled NotStarted without ever running.
	PhaseAbandoned
)

// schedule is one Task's scheduling state: its phase, the work Define
// submitted, what it must wait for, and who is waiting for it. Guarded by
// the Graph's mutex.
type schedule struct {
	phase Phase
	work  Work
	// preds are the Task's predecessors: its After arguments and, inside
	// a Sequence, the step declared just before its own.
	preds []Predecessor
	// dependents are the Tasks parked until this one resolves.
	dependents []*Task
}

// submitted reports whether Define submitted this Task's work and the
// scheduler has not abandoned it: its configuration is frozen and only the
// callback's return decides its outcome.
func (s *schedule) submitted() bool {
	return s.phase == PhaseQueued || s.phase == PhaseParked || s.phase == PhaseRunning
}

// awaitingStart reports whether the work was submitted and nobody has
// started it yet. The caller still checks the Task is not terminal.
func (s *schedule) awaitingStart() bool {
	return s.phase == PhaseQueued || s.phase == PhaseParked
}

// Predecessor is one thing a Task waits for: another Task, or a whole
// Group/Sequence. The zero Predecessor names a handle this run never
// declared, which can never succeed.
type Predecessor struct {
	task *Task
	col  *Container
	// through is, for a collection named while populated, the declaration
	// cursor at that moment: the edge reads only the members declared
	// through it. 0 for one named while empty.
	through int
}

// AfterTask is the Predecessor that waits for t. A nil t is the Predecessor
// that never succeeds.
func AfterTask(t *Task) Predecessor { return Predecessor{task: t} }

// AfterContainer is the Predecessor that waits for every member of c. A nil
// c is the Predecessor that never succeeds.
func AfterContainer(c *Container) Predecessor { return Predecessor{col: c} }

// predOutcome is what a predecessor currently tells the Tasks after it.
type predOutcome uint8

const (
	// predPending: it may still succeed.
	predPending predOutcome = iota
	predSucceeded
	// predFailed: it can never succeed, so its dependents never start.
	predFailed
)

// stateOutcome classifies a Task state for the Tasks after it.
func stateOutcome(s record.EntityState) predOutcome {
	switch s {
	case record.Done, record.Skipped:
		return predSucceeded
	case record.Failed, record.Blocked, record.Cancelled, record.NotStarted:
		return predFailed
	default:
		return predPending
	}
}

// StopsSequenceFollowers reports whether a step settling in state keeps
// every later step of its Sequence from starting: the same states that can
// never satisfy a dependent.
func StopsSequenceFollowers(state record.EntityState) bool {
	return stateOutcome(state) == predFailed
}

// Proposal is a caller's unratified success claim on a submitted Task, held
// until the callback's return value confirms or contradicts it.
type Proposal struct {
	State    record.EntityState
	Summary  string
	Problems []record.Problem
}

// Phase is where the Task stands with the scheduler.
func (t *Task) Phase() Phase {
	t.graph.lock()
	defer t.graph.unlock()
	return t.sched.phase
}

// Submitted reports whether Define submitted this Task's work and the
// scheduler has not abandoned it.
func (t *Task) Submitted() bool {
	t.graph.lock()
	defer t.graph.unlock()
	return t.sched.submitted()
}

// AwaitingStart reports whether the Task is submitted work nobody has
// started or resolved.
func (t *Task) AwaitingStart() bool {
	t.graph.lock()
	defer t.graph.unlock()
	return t.awaitingStartLocked()
}

// NeverDefined reports whether the Task is still waiting on its caller:
// declared, never Defined, and not resolved by a verb either.
func (t *Task) NeverDefined() bool {
	t.graph.lock()
	defer t.graph.unlock()
	return t.neverDefinedLocked()
}

// PredecessorCount is how many predecessors the Task still holds: a
// satisfied Task predecessor is forgotten, so the count never exceeds what
// the Task is still waiting on.
func (t *Task) PredecessorCount() int {
	t.graph.lock()
	defer t.graph.unlock()
	return len(t.sched.preds)
}

// IsGate reports whether the Task is a container builder's gate: the
// scheduler's entity for the container's deferred declaration work, no row
// and in no collection.
func (t *Task) IsGate() bool { return t.gateFor != nil }

// GateFor is the container a builder gate declares the children of, nil for
// any other Task.
func (t *Task) GateFor() *Container { return t.gateFor }

// Done is closed when the Task reaches a terminal state.
func (t *Task) Done() <-chan struct{} { return t.done }

// HasProposal reports whether a caller's success claim is being held.
func (t *Task) HasProposal() bool {
	t.graph.lock()
	defer t.graph.unlock()
	return t.proposal != nil
}

// TakeProposal hands back the held claim, if any, and forgets it.
func (t *Task) TakeProposal() *Proposal {
	t.graph.lock()
	defer t.graph.unlock()
	p := t.proposal
	t.proposal = nil
	return p
}

func (t *Task) awaitingStartLocked() bool {
	return t.sched.awaitingStart() && !record.IsTerminalTask(t.Rec.State())
}

func (t *Task) neverDefinedLocked() bool {
	return t.sched.phase == PhaseDeclared && !record.IsTerminalTask(t.Rec.State())
}

func (t *Task) closeDoneLocked() {
	t.doneOnce.Do(func() { close(t.done) })
}
