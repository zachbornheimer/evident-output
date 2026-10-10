package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// Work is what a caller submits for a Task: the callback, and the steps the
// caller takes around it. The graph runs them in a fixed order on the
// goroutine that claimed the Task and never holds its own lock while one
// runs, so a step may call back into the graph.
type Work struct {
	// Started runs once the Task was claimed, before Run, on the claiming
	// goroutine: the caller's own bookkeeping for a Task that began. It
	// reports whether the Task began. False means the Task settled between
	// its claim and this step, so it never started: Run and Observed are
	// skipped, and no callback runs for a Task that announced no start.
	Started func() bool
	// Run is the Task's callback. Its return value is recorded as the Task's
	// WorkErr before Observed hears it.
	Run func() error
	// Observed settles the Task from what Run returned. It runs only when
	// Run left the Task unsettled.
	Observed func(err error)
	// Panicked settles the Task from the text of a panic that escaped Run
	// or Observed.
	Panicked func(summary string)
}

// SubmitVerdict is how a Submit was received.
type SubmitVerdict uint8

const (
	// Submitted means the work is queued, placed, or abandoned because the
	// run was interrupted. Kick starts what may start.
	Submitted SubmitVerdict = iota
	// AlreadySettled means the Task was settled before any work was
	// submitted for it; nothing changed.
	AlreadySettled
	// AlreadySubmitted means the Task already has work; nothing changed.
	AlreadySubmitted
)

// Submit queues work as t's work and places t by what its predecessors say.
// After an interrupt nothing starts, so work submitted then settles t
// NotStarted. It does not start anything: the caller calls Kick once it
// holds none of the locks Started and Observed take.
func (g *Graph) Submit(t *Task, work Work) SubmitVerdict {
	g.lock()
	defer g.unlock()
	switch {
	case record.IsTerminalTask(t.Rec.State()):
		return AlreadySettled
	case t.sched.submitted():
		return AlreadySubmitted
	}
	g.enqueueLocked(t, work)
	if g.exec.cancelled {
		g.markNotStartedLocked(t)
		return Submitted
	}
	g.placeLocked(t, scanAll)
	return Submitted
}

// WorkErr is the error the Task's callback returned, nil before it returned
// and when it returned none. It is recorded before the Task settles from it,
// so whoever wakes on the settle reads the same value.
func (t *Task) WorkErr() error {
	t.graph.lock()
	defer t.graph.unlock()
	return t.workErr
}
