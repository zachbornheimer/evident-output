package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// NotStartedSummary is the fixed detail a Task that never ran carries: the
// text is not caller-composed, so every site that settles a Task NotStarted
// spells it the same way.
const NotStartedSummary = "not started"

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
// It also owns the evidence rule (record.Task.HonestOutcome): a
// success-class target over a Task holding a Problem settles Failed,
// whichever path asked.
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. What a projection owes a terminal
// transition it learns from the record's listener.
func (g *Graph) Settle(t *Task, state record.EntityState) {
	g.lock()
	defer g.unlock()
	g.settleLocked(t, state)
}

func (g *Graph) settleLocked(t *Task, state record.EntityState) {
	state = t.Rec.HonestOutcome(state)
	t.Rec.Settle(state)
	if t.gateFor != nil {
		g.concludeGateLocked(t)
		return
	}
	t.Rec.ClearPhase()
	t.closeDoneLocked()
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
	t.Rec.SetSummary(NotStartedSummary)
	g.settleLocked(t, record.NotStarted)
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
		c.stoppedAfter = branch
		g.stopFollowersLocked(c, branch)
	}
}

// stopFollowersLocked settles NotStarted every unstarted member of
// Sequence c's steps declared after branch.
func (g *Graph) stopFollowersLocked(c *Container, branch int) {
	for _, sib := range c.tasks {
		if sib.Declaration > branch {
			g.sched.followerChecks++
			g.stopUnstartedLocked(sib)
		}
	}
	for _, child := range c.children {
		if child.Declaration > branch {
			for _, member := range appendDescendantTasks(child, nil) {
				g.sched.followerChecks++
				g.stopUnstartedLocked(member)
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
			g.stopUnstartedLocked(t)
			return
		}
	}
}

// stopUnstartedLocked settles t NotStarted unless it already started or
// resolved.
func (g *Graph) stopUnstartedLocked(t *Task) {
	if record.IsTerminalTask(t.Rec.State()) || t.sched.phase == PhaseRunning {
		return
	}
	g.markNotStartedLocked(t)
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
