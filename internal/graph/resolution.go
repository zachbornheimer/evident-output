// This file owns how a Task resolves: the one path behind every terminal
// verb, the scheduler's own verdicts, and the run's own ending.

package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// Resolve is the one path behind every terminal verb. Admit decides what r
// does to t; Resolve then writes what the reader sees of it (the Summary,
// the Problems) and settles t, all under the run's lock. A Problem without
// its own detail gains the tail of the output t's child processes wrote, so
// the detail a callback captured needs no opt-in (beginner-2).
func (g *Graph) Resolve(t *Task, r Resolution) {
	h := g.host
	h.Resolving(t)
	h.Lock()
	defer h.Unlock()
	if h.CheckOpen() != nil {
		return
	}
	admission := g.Admit(t, r)
	if admission.Verdict == AlreadyResolved {
		h.AlreadyResolved(t, r.Summary)
		return
	}
	if disowns(t, r) {
		h.Disowned(t)
	}
	if admission.Verdict == Proposed {
		return
	}
	if r.Summary != "" {
		t.Rec.SetSummary(r.Summary)
	}
	t.Rec.ResolveProblems(admission.State, r.Problems, evidenceTail(t, admission.State, r.Problems))
	g.Conclude(t, admission)
	h.Settled(t)
}

// disowns reports whether r is t's callback disowning its own work: a
// caller's non-success verb on a running Task, from the callback's own stack.
// A later verb from the program and an interrupt both describe work that
// really happened, and are not disowning.
func disowns(t *Task, r Resolution) bool {
	return r.By == ByCaller && !r.State.Succeeded() && t.Phase() == PhaseRunning && CallbackDepth() > 0
}

// evidenceTail is the tail of t's retained output a settle into state owes
// a Problem without detail, or "" when none does. The capture is read only
// then, since reading it re-enters the redactor.
func evidenceTail(t *Task, state record.State, extra []record.Problem) string {
	if !t.Rec.NeedsEvidenceTail(state, extra) {
		return ""
	}
	return t.Rec.Captured().DetailText()
}

// observed settles t from what its callback returned: the ending the error
// names, the success the callback proposed (its words now backed by an
// observation), or plain success.
func (g *Graph) observed(t *Task, err error) {
	observation := g.Observe(t, err)
	switch observation.Kind {
	case ObservedFailure:
		state, p := g.host.Ending(err)
		g.Resolve(t, Resolution{State: state, Summary: p.Summary, Problems: []record.Problem{p}, By: ByScheduler})
	case ObservedProposal:
		proposal := observation.Proposal
		g.Resolve(t, Resolution{State: proposal.State, Summary: proposal.Summary, Problems: proposal.Problems, By: ByScheduler})
	default:
		g.Resolve(t, Resolution{State: record.Succeeded, By: ByScheduler})
	}
}

// fail settles t Failed on the scheduler's authority with summary. A code
// makes the Problem machine-matchable.
func (g *Graph) fail(t *Task, summary, code string) {
	summary = record.SanitizeText(summary)
	p := record.ApplyProblemOptions(summary, nil)
	if code != "" {
		p = record.SanitizeProblem(record.Problem{Code: code, Summary: summary})
	}
	g.Resolve(t, Resolution{State: record.Failed, Summary: summary, Problems: []record.Problem{p}, By: ByScheduler})
}

// satisfy settles t Satisfied on the scheduler's authority: what it would
// have done was already true.
func (g *Graph) satisfy(t *Task) {
	g.Resolve(t, Resolution{State: record.Satisfied, By: ByScheduler})
}

// EndRun settles the run's own ending, which no declared Task carries, on a
// Task the library invents: already settled into init's state, named name.
// It is refused, as misuse, once the run is closing.
func (g *Graph) EndRun(name string, init record.TaskInit) {
	h := g.host
	h.Lock()
	defer h.Unlock()
	if h.CheckOpen() != nil {
		return
	}
	h.RunEnded(g.AddTerminalTask(name, init))
}
