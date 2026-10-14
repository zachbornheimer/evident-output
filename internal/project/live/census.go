package live

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// census counts every Task at or below one collection by what a live frame
// ranks and tallies it on. Each Task transition moves the census of every
// collection above it, so a frame can rank and tally a collection it cannot
// show without walking it: a Group of per-item Groups costs a frame what the
// screen shows, not what the run holds (E-091, E-096).
type census struct {
	total, done, running, pending, unfinished, failed, warned int
	// unstamped counts Running or Pending Tasks no frame has painted yet
	// (row.stamp), so a frame skips a subtree with none.
	unstamped int
	// earliestSeen is the earliest first paint below; nameWidth the widest
	// Task name, in runes.
	earliestSeen time.Time
	nameWidth    int
}

// counts is the census as the tally a live projection keeps.
func (c *census) counts() project.ChildCounts {
	return project.ChildCounts{
		Total: c.total, Done: c.done,
		Running: c.running > 0, Pending: c.pending > 0, Unfinished: c.unfinished > 0,
		EarliestSeen: c.earliestSeen, NameWidth: c.nameWidth,
	}
}

// rank is the attention rank of the census' most urgent Task.
func (c *census) rank() int {
	return AttentionRank(c.failed > 0, c.warned > 0, c.running > 0, c.pending > 0)
}

// count moves the per-state counts of one Task in state by delta.
func (c *census) count(state record.State, delta int) {
	switch state {
	case record.Succeeded, record.Satisfied, record.Excluded:
		c.done += delta
	case record.Running:
		c.running += delta
	case record.Pending:
		c.pending += delta
	case record.Failed:
		c.failed += delta
	}
	if !record.IsTerminalTask(state) {
		c.unfinished += delta
	}
}

// seen folds a first-paint stamp into earliestSeen.
func (c *census) seen(at time.Time) {
	if !at.IsZero() && (c.earliestSeen.IsZero() || at.Before(c.earliestSeen)) {
		c.earliestSeen = at
	}
}

// censusesAbove calls fn on the census of every collection r sits under.
func (r *row) censusesAbove(fn func(*census)) {
	for g := r.group; g != nil; g = g.parent {
		fn(&g.census)
	}
}

// currentCensus is g's census after following the record, so it counts
// every Task by the state the record holds now.
func (g *group) currentCensus() *census {
	g.board.source.Pull()
	return &g.census
}

// censusDeclared counts a newly declared Task under the state it was
// declared in; censusSync moves it from there.
func (r *row) censusDeclared() {
	width := r.width()
	unstamped := r.unstampedIn(r.followed)
	warned := r.rec.WarningCount() > 0
	r.censusesAbove(func(c *census) {
		c.total++
		c.count(r.followed, 1)
		if warned {
			c.warned++
		}
		if unstamped {
			c.unstamped++
		}
		c.nameWidth = max(c.nameWidth, width)
		c.seen(r.seen)
	})
}

// censusSync moves r from the state the census counts it under to the state
// the record holds now. Moving by the counted state, not by the transition a
// notification named, keeps the census right however late or how often the
// record tells of a change.
func (r *row) censusSync() {
	from, to := r.followed, r.rec.State()
	if from == to {
		return
	}
	r.followed = to
	stampDelta := boolDelta(r.unstampedIn(to)) - boolDelta(r.unstampedIn(from))
	r.censusesAbove(func(c *census) {
		c.count(from, -1)
		c.count(to, 1)
		c.unstamped += stampDelta
	})
}

// censusStamped records r's first live paint.
func (r *row) censusStamped() {
	r.censusesAbove(func(c *census) {
		c.unstamped--
		c.seen(r.seen)
	})
}

func boolDelta(b bool) int {
	if b {
		return 1
	}
	return 0
}
