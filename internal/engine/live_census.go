package engine

import (
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render/live"
)

// liveCensus counts every Task at or below one collection by what a live
// frame ranks and tallies it on. Each Task transition moves the census of
// every collection above it, so a frame can rank and tally a collection it
// cannot show without walking it: a Group of per-item Groups costs a frame
// what the screen shows, not what the run holds (E-091, E-096).
//
// slice 6: one of three counters of a collection's descendants by state, with
// the graph's tally and stateCounts; they fold into one count owned by project.
type liveCensus struct {
	total, done, running, pending, unfinished, failed, warned int
	// unstamped counts Running or Pending Tasks no frame has painted yet
	// (taskState.stampLiveFirstSeen), so a frame skips a subtree with none.
	unstamped int
	// earliestSeen is the earliest liveFirstSeenAt below; nameWidth the
	// widest Task name, in runes.
	earliestSeen time.Time
	nameWidth    int
}

// counts is the census as the tally a live projection keeps.
func (c *liveCensus) counts() core.ChildCounts {
	return core.ChildCounts{
		Total: c.total, Done: c.done,
		Running: c.running > 0, Pending: c.pending > 0, Unfinished: c.unfinished > 0,
		EarliestSeen: c.earliestSeen, NameWidth: c.nameWidth,
	}
}

// rank is the attention rank of the census' most urgent Task.
func (c *liveCensus) rank() int {
	return live.AttentionRank(c.failed > 0, c.warned > 0, c.running > 0, c.pending > 0)
}

// count moves the per-state counts of one Task in state by delta.
func (c *liveCensus) count(state EntityState, delta int) {
	switch state {
	case Done, Skipped:
		c.done += delta
	case Running:
		c.running += delta
	case Pending:
		c.pending += delta
	case Failed:
		c.failed += delta
	}
	if !core.IsTerminalTask(state) {
		c.unfinished += delta
	}
}

// seen folds a liveFirstSeenAt stamp into earliestSeen.
func (c *liveCensus) seen(at time.Time) {
	if !at.IsZero() && (c.earliestSeen.IsZero() || at.Before(c.earliestSeen)) {
		c.earliestSeen = at
	}
}

// censusesAbove calls fn on the census of every collection t sits under.
func (t *taskState) censusesAbove(fn func(*liveCensus)) {
	for c := t.collection(); c != nil; c = c.parent() {
		fn(&c.census)
	}
}

// unstamped reports whether t, in state, is a live row no frame painted.
func (t *taskState) unstampedIn(state EntityState) bool {
	return (state == Running || state == Pending) && t.liveFirstSeenAt.IsZero()
}

// currentCensus is g's census after following the record, so it counts
// every Task by the state the record holds now.
func (g *tasksState) currentCensus() *liveCensus {
	g.out.pullRecordLocked()
	return &g.census
}

// censusDeclared counts a newly declared Task under the state it was
// declared in; censusSync moves it from there.
func (t *taskState) censusDeclared() {
	width := utf8.RuneCountInString(t.name)
	unstamped := t.unstampedIn(t.followed)
	t.censusesAbove(func(c *liveCensus) {
		c.total++
		c.count(t.followed, 1)
		if t.node.Rec.WarningCount() > 0 {
			c.warned++
		}
		if unstamped {
			c.unstamped++
		}
		c.nameWidth = max(c.nameWidth, width)
		c.seen(t.liveFirstSeenAt)
	})
}

// censusSync moves t from the state the census counts it under to the state
// the record holds now. Moving by the counted state, not by the transition a
// notification named, keeps the census right however late or how often the
// record tells of a change.
func (t *taskState) censusSync() {
	from, to := t.followed, t.node.Rec.State()
	if from == to {
		return
	}
	t.followed = to
	stampDelta := boolDelta(t.unstampedIn(to)) - boolDelta(t.unstampedIn(from))
	t.censusesAbove(func(c *liveCensus) {
		c.count(from, -1)
		c.count(to, 1)
		c.unstamped += stampDelta
	})
}

// censusStamped records t's first live paint.
func (t *taskState) censusStamped() {
	t.censusesAbove(func(c *liveCensus) {
		c.unstamped--
		c.seen(t.liveFirstSeenAt)
	})
}

// censusWarned records t's first warning.
func (t *taskState) censusWarned() {
	t.censusesAbove(func(c *liveCensus) { c.warned++ })
}

func boolDelta(b bool) int {
	if b {
		return 1
	}
	return 0
}

// liveOwnRow is the name g's one row renders under when it renders as its
// own Task, or "". Only a Group holding a Task of its own name can, so
// every other Group answers without being walked.
func (g *tasksState) liveOwnRow() string {
	if !g.hasNamesake {
		return ""
	}
	return live.OwnRowName(g.view())
}
