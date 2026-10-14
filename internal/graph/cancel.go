// This file owns what an interrupt cancels before the graph abandons the
// rest: the pending Confirm gate, else the running Tasks, else the next one
// about to run, else the run itself.

package graph

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// CancelActive cancels what the run is doing, so an interrupt always leaves
// a typed Cancelled state. A pending Confirm gate goes first: it holds sole
// control of the run, and closing its abort channel is what unblocks its
// stdin read. Then every running Task; with none, the earliest-declared
// pending one, the one about to run next, so an interrupt before any
// evidence still cancels a Task; with none, the run itself. It takes the
// run's lock itself.
func (g *Graph) CancelActive(reason string) {
	h := g.host
	h.Lock()
	if g.CancelPendingGate(reason) {
		h.Unlock()
		return
	}
	targets := g.activeTasks()
	h.Unlock()
	for _, t := range targets {
		g.Resolve(t, Resolution{State: record.Cancelled, Summary: record.SanitizeText(reason), By: ByCaller})
	}
	if len(targets) == 0 {
		g.EndRun(h.EndingName(), record.TaskInit{State: record.Cancelled, Summary: record.SanitizeText(reason)})
	}
}

// activeTasks are the running Tasks, in declaration order, or else the
// earliest-declared pending one.
func (g *Graph) activeTasks() []*Task {
	g.lockRead()
	tasks := g.taskList
	g.unlockRead()
	var running []*Task
	for _, t := range tasks {
		if t.Rec.State() == record.Running {
			running = append(running, t)
		}
	}
	if len(running) > 0 {
		return running
	}
	for _, t := range tasks {
		if t.Rec.State() == record.Pending {
			return []*Task{t}
		}
	}
	return nil
}

// CancelPendingGate cancels one pending Confirm gate, if any, so ^C at a
// "[y/N]" prompt unblocks its stdin read and settles it Cancelled, never
// Blocked "declined": a human "n" and an interrupt are distinct outcomes.
// It reports whether a gate was cancelled. The caller holds the run's lock.
func (g *Graph) CancelPendingGate(reason string) bool {
	id, ok := g.AbortPending()
	if !ok {
		return false
	}
	if t := g.Task(id); t != nil && !terminal(t) {
		t.Rec.SetSummary(reason)
		g.Settle(t, record.Cancelled)
		g.host.Settled(t)
	}
	return true
}
