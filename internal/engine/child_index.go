package engine

import (
	"container/heap"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	"github.com/zachbornheimer/evident-output/internal/render/live"
)

// childIndex keeps what a live frame needs of one collection's direct
// Tasks current as they change, so a frame reads the children it can show
// and the counts of the rest without walking them: a frame costs what the
// screen shows, however many Tasks the collection holds (E-091).
//
// A Task that changed is marked (taskState.markFiling) and refiled at the
// next read, so a burst of Progress calls on one Task refiles it once. The
// index files each child the way live.ClassifyChild does, so the frame it
// yields equals the one live.LiveChildren builds by walking every child.
type childIndex struct {
	dirty []*taskState

	// items are the disposition items and work every other child, by
	// declaration; byRank splits work by attention rank.
	items, work positionSet
	byRank      [live.AttentionRanks]positionSet

	all, workCounts childCounters
	states          stateCounts
	seenAll         seenHeap
	seenWork        seenHeap
	owns, peers     int

	// itemDispositions is the items' tally as of the items in [0,
	// itemsThrough]; itemsStale means a change it cannot append to.
	itemDispositions core.Dispositions
	itemsThrough     int
	itemsStale       bool

	// unstamped are Tasks filed while Running or Pending that no frame
	// has painted yet (taskState.stampLiveFirstSeen).
	unstamped []*taskState
}

// filing is how one child Task is filed in its collection's childIndex.
type filing struct {
	filed, dirty, listed bool
	// pos is the Task's declaration position among its collection's Tasks.
	pos  int
	kind live.ChildKind
	// state, width, seen, and warned are the Task fields the counts read.
	state  EntityState
	width  int
	seen   time.Time
	warned bool
	// allSeenAt and workSeenAt are the stamps last pushed to the seen heaps.
	allSeenAt, workSeenAt time.Time
}

// markFiling records that t changed in a way its collection's frame or
// verdict reads, so the next read refiles it. Call it after any change to
// a taskState field the collection's frame shows or counts.
func (t *taskState) markFiling() {
	col := t.collection
	if col == nil || t.filing.dirty {
		return
	}
	t.filing.dirty = true
	col.kids.dirty = append(col.kids.dirty, t)
}

// settled is g's index with every marked Task refiled.
func (g *tasksState) settled() *childIndex {
	x := &g.kids
	for len(x.dirty) > 0 {
		dirty := x.dirty
		x.dirty = nil
		for _, t := range dirty {
			x.refile(g, t)
		}
	}
	return x
}

func (x *childIndex) refile(g *tasksState, t *taskState) {
	old := t.filing
	view := t.view()
	next := filing{
		filed: true, pos: old.pos, listed: old.listed,
		kind:  live.ClassifyChild(g.name, &view),
		state: t.state, width: utf8.RuneCountInString(t.name),
		seen: t.liveFirstSeenAt, warned: len(t.warnings) > 0,
		allSeenAt: old.allSeenAt, workSeenAt: old.workSeenAt,
	}
	if old.filed {
		if next.sameFiling(old) && !next.kind.Item {
			t.filing = next
			return
		}
		x.unfile(t)
		next.workSeenAt = time.Time{}
	}
	t.filing = next
	x.file(g, t, &view)
}

func (f filing) sameFiling(o filing) bool {
	return f.kind == o.kind && f.state == o.state && f.width == o.width &&
		f.seen.Equal(o.seen) && f.warned == o.warned
}

func (x *childIndex) file(g *tasksState, t *taskState, view *core.TaskSnapshot) {
	f := &t.filing
	x.all.add(f)
	x.states.add(f)
	if f.kind.OwnTask {
		x.owns++
	}
	if !f.seen.IsZero() && !f.seen.Equal(f.allSeenAt) {
		f.allSeenAt = f.seen
		heap.Push(&x.seenAll, seenEntry{at: f.seen, t: t})
	}
	if f.kind.Item {
		x.items.add(f.pos)
		x.fileItem(f.pos, view)
	} else {
		x.work.add(f.pos)
		x.workCounts.add(f)
		if f.kind.Rank < live.AttentionRanks {
			x.byRank[f.kind.Rank].add(f.pos)
		}
		if f.kind.WorkPeer {
			x.peers++
		}
		if !f.seen.IsZero() && !f.seen.Equal(f.workSeenAt) {
			f.workSeenAt = f.seen
			heap.Push(&x.seenWork, seenEntry{at: f.seen, t: t})
		}
	}
	if t.unstampedIn(f.state) && !f.listed {
		f.listed = true
		x.unstamped = append(x.unstamped, t)
	}
}

func (x *childIndex) unfile(t *taskState) {
	f := &t.filing
	x.all.remove(f)
	x.states.remove(f)
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
	if f.kind.Rank < live.AttentionRanks {
		x.byRank[f.kind.Rank].remove(f.pos)
	}
	if f.kind.WorkPeer {
		x.peers--
	}
}

// fileItem adds the item at pos to the items' tally: appended when it
// follows every item already in it, since the tally is in declaration
// order, otherwise the tally is rebuilt at the next read.
func (x *childIndex) fileItem(pos int, view *core.TaskSnapshot) {
	if x.itemsStale || pos < x.itemsThrough {
		x.itemsStale = true
		return
	}
	x.itemDispositions.AddTask(view)
	x.itemsThrough = pos
}

func (x *childIndex) dispositions(g *tasksState) core.Dispositions {
	if x.itemsStale {
		x.itemDispositions, x.itemsThrough, x.itemsStale = core.Dispositions{}, -1, false
		x.items.each(func(pos int) {
			view := g.tasks[pos].view()
			x.itemDispositions.AddTask(&view)
			x.itemsThrough = pos
		})
	}
	return x.itemDispositions.Snapshot()
}

// stampDirectTasks stamps the Tasks filed while no frame had painted
// them, as a frame that counted them does.
func (g *tasksState) stampDirectTasks(now time.Time) {
	x := g.settled()
	listed := x.unstamped
	x.unstamped = nil
	for _, t := range listed {
		t.filing.listed = false
		t.stampLiveFirstSeen(now)
	}
}

// project is the children a frame of rows rows could show, snapshotted in
// declaration order, and the roster of all of them.
func (x *childIndex) project(g *tasksState, rows int) ([]core.TaskSnapshot, live.ChildRoster) {
	budget := live.ChildRows(rows)
	pos := x.items.appendFirst(nil, budget+1)
	pos = x.work.appendFirst(pos, budget+1)
	for r := range x.byRank {
		pos = x.byRank[r].appendFirst(pos, budget)
	}
	slices.Sort(pos)
	pos = slices.Compact(pos)
	var kept []core.TaskSnapshot
	for _, p := range pos {
		kept = append(kept, g.tasks[p].snapshot())
	}
	return kept, live.ChildRoster{
		All:    x.all.counts(x.seenAll.earliest(isFiled)),
		Work:   x.workCounts.counts(x.seenWork.earliest(isWork)),
		Items:  x.dispositions(g),
		Census: render.ChildCensus{Items: x.items.len(), OwnTask: x.owns > 0, WorkPeer: x.peers > 0},
	}
}

func isFiled(t *taskState) bool { return t.filing.filed }

func isWork(t *taskState) bool { return t.filing.filed && !t.filing.kind.Item }
