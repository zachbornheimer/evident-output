package live

import (
	"container/heap"
	"slices"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// childIndex keeps what a live frame needs of one collection's direct
// Tasks current as they change, so a frame reads the children it can show
// and the counts of the rest without walking them: a frame costs what the
// screen shows, however many Tasks the collection holds (E-091).
//
// A Task that changed is marked (row.markFiling) and refiled at the next
// read, so a burst of Progress calls on one Task refiles it once. The
// index files each child the way ClassifyChild does, so the frame it
// yields equals the one LiveChildren builds by walking every child.
type childIndex struct {
	dirty []*row

	// items are the Excluded items and work every other child, by
	// declaration; byRank splits work by attention rank.
	items, work positionSet
	byRank      [AttentionRanks]positionSet

	all, workCounts childCounters
	states          project.StateCounts
	seenAll         seenHeap
	seenWork        seenHeap
	owns            int

	// itemTally is the items' tally as of the items in [0,
	// itemsThrough]; itemsStale means a change it cannot append to.
	itemTally    project.Tally
	itemsThrough int
	itemsStale   bool

	// unstamped are Tasks filed while Running or Pending that no frame
	// has painted yet (row.stamp).
	unstamped []*row
}

// filing is how one child Task is filed in its collection's childIndex.
type filing struct {
	filed, dirty, listed bool
	// pos is the Task's declaration position among its collection's Tasks.
	pos  int
	kind ChildKind
	// state, width and seen are the Task fields the counts read.
	state record.State
	width int
	seen  time.Time
	// allSeenAt and workSeenAt are the stamps last pushed to the seen heaps.
	allSeenAt, workSeenAt time.Time
}

// markFiling records that r changed in a way its collection's frame or
// verdict reads, so the next read refiles it.
func (r *row) markFiling() {
	g := r.group
	if g == nil || r.filing.dirty {
		return
	}
	r.filing.dirty = true
	g.kids.dirty = append(g.kids.dirty, r)
}

// settled is g's index with every marked Task refiled.
func (g *group) settled() *childIndex {
	g.board.source.Pull()
	x := &g.kids
	for len(x.dirty) > 0 {
		dirty := x.dirty
		x.dirty = nil
		for _, r := range dirty {
			x.refile(g, r)
		}
	}
	return x
}

func (x *childIndex) refile(g *group, r *row) {
	old := r.filing
	view := g.board.source.View(r.id)
	next := filing{
		filed: true, pos: old.pos, listed: old.listed,
		kind:  ClassifyChild(g.name, &view),
		state: r.rec.State(), width: r.width(),
		seen:      r.seen,
		allSeenAt: old.allSeenAt, workSeenAt: old.workSeenAt,
	}
	if old.filed {
		if next.sameFiling(old) && !next.kind.Item {
			r.filing = next
			return
		}
		x.unfile(r)
		next.workSeenAt = time.Time{}
	}
	r.filing = next
	x.file(r, &view)
}

func (f filing) sameFiling(o filing) bool {
	return f.kind == o.kind && f.state == o.state && f.width == o.width &&
		f.seen.Equal(o.seen)
}

func (x *childIndex) file(r *row, view *record.TaskSnapshot) {
	f := &r.filing
	x.all.add(f)
	countStates(&x.states, f, 1)
	if f.kind.OwnTask {
		x.owns++
	}
	if !f.seen.IsZero() && !f.seen.Equal(f.allSeenAt) {
		f.allSeenAt = f.seen
		heap.Push(&x.seenAll, seenEntry{at: f.seen, r: r})
	}
	if f.kind.Item {
		x.items.add(f.pos)
		x.fileItem(f.pos, view)
	} else {
		x.work.add(f.pos)
		x.workCounts.add(f)
		if f.kind.Rank < AttentionRanks {
			x.byRank[f.kind.Rank].add(f.pos)
		}
		if !f.seen.IsZero() && !f.seen.Equal(f.workSeenAt) {
			f.workSeenAt = f.seen
			heap.Push(&x.seenWork, seenEntry{at: f.seen, r: r})
		}
	}
	if r.unstampedIn(f.state) && !f.listed {
		f.listed = true
		x.unstamped = append(x.unstamped, r)
	}
}

func (x *childIndex) unfile(r *row) {
	f := &r.filing
	x.all.remove(f)
	countStates(&x.states, f, -1)
	if f.kind.OwnTask {
		x.owns--
	}
	if f.kind.Item {
		x.items.remove(f.pos)
		x.itemsStale = true
		return
	}
	x.work.remove(f.pos)
	x.workCounts.remove(f)
	if f.kind.Rank < AttentionRanks {
		x.byRank[f.kind.Rank].remove(f.pos)
	}
}

// fileItem adds the item at pos to the items' tally: appended when it
// follows every item already in it, since the tally is in declaration
// order, otherwise the tally is rebuilt at the next read.
func (x *childIndex) fileItem(pos int, view *record.TaskSnapshot) {
	if x.itemsStale || pos < x.itemsThrough {
		x.itemsStale = true
		return
	}
	x.itemTally.AddItem(view)
	x.itemsThrough = pos
}

func (x *childIndex) excluded(g *group) project.Tally {
	if x.itemsStale {
		x.itemTally, x.itemsThrough, x.itemsStale = project.Tally{}, -1, false
		x.items.each(func(pos int) {
			view := g.board.source.View(g.tasks[pos].id)
			x.itemTally.AddItem(&view)
			x.itemsThrough = pos
		})
	}
	return x.itemTally.Clone()
}

// stampDirect stamps the Tasks filed while no frame had painted them, as
// a frame that counted them does.
func (g *group) stampDirect(now time.Time) {
	x := g.settled()
	listed := x.unstamped
	x.unstamped = nil
	for _, r := range listed {
		r.filing.listed = false
		r.stamp(now)
	}
}

// project is the children a frame of rows rows could show, snapshotted in
// declaration order, and the roster of all of them.
func (x *childIndex) project(g *group, rows int) ([]record.TaskSnapshot, ChildRoster) {
	budget := ChildRows(rows)
	pos := x.items.appendFirst(nil, budget+1)
	pos = x.work.appendFirst(pos, budget+1)
	for r := range x.byRank {
		pos = x.byRank[r].appendFirst(pos, budget)
	}
	slices.Sort(pos)
	pos = slices.Compact(pos)
	var kept []record.TaskSnapshot
	for _, p := range pos {
		kept = append(kept, g.board.source.Snapshot(g.tasks[p].id))
	}
	return kept, ChildRoster{
		All:    x.all.counts(x.seenAll.earliest(isFiled)),
		Work:   x.workCounts.counts(x.seenWork.earliest(isWork)),
		Items:  x.excluded(g),
		Census: project.ChildCensus{Items: x.items.len(), OwnTask: x.owns > 0},
	}
}

func isFiled(r *row) bool { return r.filing.filed }

func isWork(r *row) bool { return r.filing.filed && !r.filing.kind.Item }
