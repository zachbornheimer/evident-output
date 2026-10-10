package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// Authority names who is resolving a Task: the program that wrote a terminal
// verb, or the scheduler reporting what the Task's callback returned.
type Authority uint8

const (
	// ByCaller is a verb the program wrote by hand.
	ByCaller Authority = iota
	// ByScheduler is the scheduler's own report of what the callback
	// returned. It is the sole authority that may declare submitted work
	// successful.
	ByScheduler
)

// Resolution is what an authority asks of a Task: the state it should reach,
// the words that say why, and the Problems it found.
type Resolution struct {
	State    record.EntityState
	Summary  string
	Problems []record.Problem
	By       Authority
}

// Verdict is what Admit decided about a Resolution.
type Verdict uint8

const (
	// Settling means the Task will reach Admission.State: the caller writes
	// the Summary and Problems, then calls Conclude.
	Settling Verdict = iota
	// Proposed means the Resolution was held, not applied: a submitted Task
	// takes its success only from its callback's return value.
	Proposed
	// AlreadyResolved means the Task was terminal before the Resolution;
	// nothing changed.
	AlreadyResolved
)

// Admission is Admit's answer, handed back to Conclude.
type Admission struct {
	// Verdict is what the graph decided.
	Verdict Verdict
	// State is, for Settling, the state the Task reaches: the evidence rule
	// has already turned a success over a Problem into Failed.
	State record.EntityState

	stopsFollowers bool
}

// Admit decides what a Resolution does to t. Once a Task is submitted, the
// scheduler owns the verdict on its work: only the callback's own return
// value says whether the work succeeded. So a success a caller asks for on a
// submitted Task is a proposal, held until the callback returns and then
// either ratified or contradicted (see Task.TakeProposal). Nothing ratifies
// its own completion.
//
// Bad news needs no ratification: Fail, Block and Cancel state an outcome the
// caller already knows and can only make the row worse, so they settle at
// once, as does every Resolution the scheduler itself makes.
//
// A Settling Task is not settled yet: the caller writes what differs between
// resolutions (its Summary and Problems), then calls Conclude.
func (g *Graph) Admit(t *Task, r Resolution) Admission {
	g.lock()
	defer g.unlock()
	switch {
	case record.IsTerminalTask(t.Rec.State()):
		return Admission{Verdict: AlreadyResolved}
	case t.sched.submitted() && r.By == ByCaller && record.DeclaresSuccess(r.State):
		t.proposal = &Proposal{State: r.State, Summary: r.Summary, Problems: r.Problems}
		return Admission{Verdict: Proposed}
	}
	state := t.Rec.HonestOutcome(r.State)
	return Admission{
		Verdict:        Settling,
		State:          state,
		stopsFollowers: t.sched.submitted() && StopsSequenceFollowers(state),
	}
}

// Conclude settles t as Admit decided, and settles NotStarted the steps its
// failure keeps from running, in the same critical section: whoever sees t
// terminal sees its followers settled too.
func (g *Graph) Conclude(t *Task, a Admission) {
	if a.Verdict != Settling {
		return
	}
	g.lock()
	defer g.unlock()
	g.settleLocked(t, a.State)
	if a.stopsFollowers {
		g.failSequenceFollowersLocked(t)
	}
}
