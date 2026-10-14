// This file owns how a Task that broke a Run-wide rule after its own work
// finished is failed: the rule is only visible once a different Task acts, so
// the Task may already have settled Done, or be about to.

package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// Overturn fails t with err, wherever t stands. A Task that already settled
// Done settles again as Failed, through the one settle path; a Task that has
// not settled keeps err and fails with it when the scheduler would have
// settled it Done. Either way both Tasks of a same-output conflict fail, in
// whichever order they ran.
//
// The keep and the Done verdict are both decided under the Run's lock, so a
// Task cannot settle Done between Overturn's check and its keep.
func (g *Graph) Overturn(t *Task, err error) {
	h := g.host
	h.Lock()
	defer h.Unlock()
	g.lock()
	switch state := t.Rec.State(); {
	case !record.IsTerminalTask(state):
		t.overturn.keep(err)
		g.unlock()
	case state.Succeeded():
		g.resettleFailedLocked(t, err)
		g.unlock()
		h.Overturned(t)
	default:
		g.unlock()
	}
}

// resettleFailedLocked rewrites t's Done verdict as the Failed that err
// names.
func (g *Graph) resettleFailedLocked(t *Task, err error) {
	state, p := g.host.Ending(err)
	t.Rec.SetSummary(p.Summary)
	t.Rec.ResolveProblems(state, []record.Problem{p}, "")
	g.settleLocked(t, state)
}

// overturnedLocked is r, or, when r would settle t Done after Overturn kept
// an error against it, the Failure that error names.
func (g *Graph) overturnedLocked(t *Task, r Resolution) Resolution {
	err := t.overturn.held()
	if err == nil || r.By != ByScheduler || !record.DeclaresSuccess(r.State) {
		return r
	}
	state, p := g.host.Ending(err)
	return Resolution{State: state, Summary: p.Summary, Problems: []record.Problem{p}, By: ByScheduler}
}
