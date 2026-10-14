package graph

import (
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// NotStartedSummary is the fixed detail a Task that never ran carries: the
// text is not caller-composed, so every site that settles a Task NotStarted
// spells it the same way. A Task a predecessor stopped adds who stopped it
// (see stoppedBy).
const NotStartedSummary = "not started"

// stoppedBy is the summary of a Task that never ran because the Task named
// name settled in state, which no dependent accepts.
func stoppedBy(name string, state record.State) string {
	return NotStartedSummary + " — " + name + " " + strings.ReplaceAll(string(state), "_", " ")
}

// Settle is the one way a Task reaches a terminal state. Every path that
// ends a Task (a caller's verb, a scheduled callback's return, a failed
// predecessor's cascade, a cancelled Confirm, a duplicate declaration,
// Finish's sweep) goes through it, so the bookkeeping a terminal transition
// owes cannot drift between sites:
//
//   - the active phase clears;
//   - every Wait parked on the Task wakes (its Done channel closes);
//   - submitted work that never started releases its hold on the drain;
//   - its collections' tallies move, and every Task parked on it (or on a
//     collection it just resolved) is placed again (see wakeLocked).
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. What a projection owes a terminal
// transition it learns from the record's listener.
func (g *Graph) Settle(t *Task, state record.State) {
	g.lock()
	defer g.unlock()
	g.settleLocked(t, state)
}

func (g *Graph) settleLocked(t *Task, state record.State) {
	t.Rec.Settle(state)
	if t.gateFor != nil {
		g.concludeGateLocked(t)
		return
	}
	t.Rec.ClearPhase()
	if t.sched.awaitingStart() {
		g.abandonLocked(t)
	}
	g.propagateSettleLocked(t)
}

// MarkNotStarted settles t NotStarted: work that will now never run.
func (g *Graph) MarkNotStarted(t *Task) {
	g.lock()
	defer g.unlock()
	g.markNotStartedLocked(t)
}

func (g *Graph) markNotStartedLocked(t *Task) {
	g.markStoppedLocked(t, NotStartedSummary)
}

// markStoppedLocked settles t NotStarted with summary, which says what
// stopped it.
func (g *Graph) markStoppedLocked(t *Task, summary string) {
	t.Rec.SetSummary(summary)
	g.settleLocked(t, record.NotStarted)
}

// stopperLocked is the summary of a Task that failed predecessor p keeps
// from running: it names the Task that can never succeed, the earliest such
// member of a collection, or a collection whose builder never declared its
// members.
func (g *Graph) stopperLocked(p Predecessor) string {
	switch {
	case p.task != nil:
		return stoppedBy(p.task.Name, p.task.Rec.State())
	case p.col == nil:
		return NotStartedSummary
	case p.col.tally.stoppedBuilder != nil:
		return p.col.tally.stoppedBuilder.summary(p.col.Name)
	}
	if m := p.col.tally.firstFailedMember(); m != nil {
		return stoppedBy(m.Name, m.Rec.State())
	}
	return NotStartedSummary
}

// abandonLocked releases the scheduler's hold on submitted work that
// settled before it ever started.
func (g *Graph) abandonLocked(t *Task) {
	g.enterPhaseLocked(t, PhaseAbandoned)
	g.sched.work.Done()
}

// FailSequenceFollowers settles NotStarted every step declared after the
// one failed belongs to, in each Sequence it sits under, that has not
// started, Defined or not. A nested Group/Sequence step settles all of its
// members.
//
// Each Sequence remembers the earliest step it already stopped after. A
// failure at or after that step finds nothing new to stop, here or in the
// Sequences above, so k failing members of one step cost one walk, not k.
func (g *Graph) FailSequenceFollowers(failed *Task) {
	g.lock()
	defer g.unlock()
	g.failSequenceFollowersLocked(failed)
}

func (g *Graph) failSequenceFollowersLocked(failed *Task) {
	branch := failed.Declaration
	for c := failed.Parent; c != nil; branch, c = c.Declaration, c.Parent {
		if !c.Sequential {
			continue
		}
		if c.stoppedAfter != 0 && branch >= c.stoppedAfter {
			return
		}
		c.stoppedAfter, c.stoppedBy = branch, stoppedBy(failed.Name, failed.Rec.State())
		g.stopFollowersLocked(c, branch)
	}
}

// stopFollowersLocked settles NotStarted every unstarted member of
// Sequence c's steps declared after branch.
func (g *Graph) stopFollowersLocked(c *Container, branch int) {
	for _, sib := range c.tasks {
		if sib.Declaration > branch {
			g.sched.followerChecks++
			g.stopUnstartedLocked(sib, c.stoppedBy)
		}
	}
	for _, child := range c.children {
		if child.Declaration > branch {
			for _, member := range appendDescendantTasks(child, nil) {
				g.sched.followerChecks++
				g.stopUnstartedLocked(member, c.stoppedBy)
			}
		}
	}
}

// stopIfFollowerLocked settles NotStarted a Task declared after a failure
// already stopped the Sequence step it belongs to: stopFollowersLocked ran
// before the Task existed, so nothing else would ever settle it.
func (g *Graph) stopIfFollowerLocked(t *Task) {
	branch := t.Declaration
	for c := t.Parent; c != nil; branch, c = c.Declaration, c.Parent {
		if c.Sequential && c.stoppedAfter != 0 && branch > c.stoppedAfter {
			g.stopUnstartedLocked(t, c.stoppedBy)
			return
		}
	}
}

// stopUnstartedLocked settles t NotStarted with summary unless it already
// started or resolved.
func (g *Graph) stopUnstartedLocked(t *Task, summary string) {
	if record.IsTerminalTask(t.Rec.State()) || t.sched.phase == PhaseRunning {
		return
	}
	g.markStoppedLocked(t, summary)
}

// FollowerChecks counts the Sequence followers examined, so a test can prove
// repeated failures stay linear.
func (g *Graph) FollowerChecks() int {
	g.lock()
	defer g.unlock()
	return g.sched.followerChecks
}

// appendDescendantTasks appends every Task declared directly or
// transitively under c, c's own first.
func appendDescendantTasks(c *Container, out []*Task) []*Task {
	out = append(out, c.tasks...)
	for _, child := range c.children {
		out = appendDescendantTasks(child, out)
	}
	return out
}
