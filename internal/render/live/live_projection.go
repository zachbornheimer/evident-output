package live

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// LiveChildren projects one collection's child Tasks for a live frame of
// at most rows rows. A frame never shows more child rows than it has
// rows, and it picks them in attention order (selectLiveChildren), so of
// a collection larger than that only the first rows children of each
// attention rank, and the first rows+1 of the rest, can ever appear. The
// projection keeps those and tallies every child, so building and
// painting a frame costs what the screen shows, not what the run holds.
//
// Feed it every child in declaration order: Admit classifies a child from
// a cheap view, and Keep takes the full snapshot of each admitted one.
// A collection small enough to keep whole projects exactly as it would
// unprojected, with no tally. A larger one paints the same frame, with
// one difference: a flattened row's name is qualified by its container
// path only when it collides with a row the frame could show, not with
// one the projection left out (qualify.go: "another visible row").
type LiveChildren struct {
	group string
	rows  int
	kept  []core.TaskSnapshot

	keptItems, keptWork int
	keptByRank          [attentionRankCount]int

	roster ChildRoster
}

// ChildRoster is what a live projection knows about every child Task of
// a collection, admitted or not: everything the tally of the ones it left
// out is built from. LiveChildren counts it in one pass; a caller that
// keeps the counts current as Tasks change (the engine's child index)
// supplies it directly to Project.
type ChildRoster struct {
	// All counts every child; Work every child that is not a disposition
	// item.
	All, Work core.ChildCounts
	// Items sums the disposition items' Skipped and Kept records.
	Items  core.Dispositions
	Census render.ChildCensus
}

// ChildKind is how a live frame files one child Task of the collection
// named group.
type ChildKind struct {
	// Item reports a disposition item: counted in its collection's
	// tally, never ranked.
	Item bool
	// OwnTask and WorkPeer feed the census that decides whether the
	// items fold (render.ChildCensus).
	OwnTask, WorkPeer bool
	// Rank is the attention rank of a non-item child; AttentionRanks or
	// more is routine.
	Rank int
}

// AttentionRanks is the rank of a child that never fills a frame's rows
// by attention.
const AttentionRanks = attentionRankCount

// ClassifyChild files t under the collection named group.
func ClassifyChild(group string, t *core.TaskSnapshot) ChildKind {
	switch {
	case render.IsOwnTask(group, t):
		return ChildKind{OwnTask: true, Rank: liveRank(*t)}
	case render.IsDispositionItem(group, t):
		return ChildKind{Item: true}
	default:
		return ChildKind{WorkPeer: render.IsWorkPeer(t), Rank: liveRank(*t)}
	}
}

// ChildRows is how many rows of children a frame of rows rows admits by
// attention rank; it admits one more, in declaration order, of each of
// the disposition items and the other children.
func ChildRows(rows int) int { return liveHeight(rows) }

// NewLiveChildren starts the projection of the children of the collection
// named group for a frame of rows rows.
func NewLiveChildren(group string, rows int) *LiveChildren {
	return &LiveChildren{group: group, rows: liveHeight(rows)}
}

// Admit counts t and reports whether the frame could show it, in which
// case the caller passes its full snapshot to Keep.
func (c *LiveChildren) Admit(t *core.TaskSnapshot) bool {
	r := &c.roster
	r.All.Add(t)
	kind := ClassifyChild(c.group, t)
	switch {
	case kind.Item:
		r.Census.Items++
		r.Items.AddTask(t)
		c.keptItems++
		return c.keptItems <= c.rows+1
	case kind.OwnTask:
		r.Census.OwnTask = true
	case kind.WorkPeer:
		r.Census.WorkPeer = true
	}
	r.Work.Add(t)
	c.keptWork++
	admit := c.keptWork <= c.rows+1
	if kind.Rank < attentionRankCount {
		c.keptByRank[kind.Rank]++
		admit = admit || c.keptByRank[kind.Rank] <= c.rows
	}
	return admit
}

// Keep adds an admitted child's full snapshot.
func (c *LiveChildren) Keep(t core.TaskSnapshot) { c.kept = append(c.kept, t) }

// Collection is col, whose own fields the caller filled, holding the kept
// children and, when some were left out, the tally of all of them.
func (c *LiveChildren) Collection(col core.TasksSnapshot) core.TasksSnapshot {
	return c.roster.Project(col, c.kept)
}

// Project is col holding kept, the children a frame could show, and,
// when some were left out, the tally of all of them.
func (r ChildRoster) Project(col core.TasksSnapshot, kept []core.TaskSnapshot) core.TasksSnapshot {
	col.Tasks = kept
	if len(kept) == r.All.Total {
		return core.WithoutChildTally(col)
	}
	tally := core.ChildTally{All: r.All, Rest: r.All}
	if !col.Sequential && r.Census.Folds(col.Summary) {
		tally.Folded, tally.Items, tally.Rest = true, r.Items, r.Work
	}
	return core.WithChildTally(col, tally)
}

// LiveCollections projects one collection's child collections for a live
// frame of at most rows rows. A frame paints nested collections in
// declaration order while they fit, and otherwise only the ones holding a
// Task that needs attention, in attention order (liveFill.groups). So only
// the first rows+1, and the first rows of each attention rank, can ever
// appear: the projection keeps those, each projected in turn, and tallies
// the rest from counts. Building a frame then costs what the screen
// shows, however many per-item collections the run holds (E-091).
type LiveCollections struct {
	rows   int
	seen   int
	byRank [attentionRankCount]int
	kept   []core.TasksSnapshot
	tally  core.CollectionTally
}

// NewLiveCollections starts the projection of a collection's child
// collections for a frame of rows rows.
func NewLiveCollections(rows int) *LiveCollections {
	return &LiveCollections{rows: liveHeight(rows)}
}

// Admit reports whether the frame could reach the next child collection,
// in declaration order, whose most urgent Task has rank (AttentionRank).
// The caller passes an admitted one's live projection to Keep and any
// other's counts to Omit.
func (c *LiveCollections) Admit(rank int) bool {
	c.seen++
	admit := c.seen <= c.rows+1
	if rank < attentionRankCount {
		c.byRank[rank]++
		admit = admit || c.byRank[rank] <= c.rows
	}
	return admit
}

// AttentionRank is the rank Admit takes for a collection whose Tasks, at
// any depth, include a failed, a warned, a running or a pending one: the
// most urgent that applies, as liveRank orders a single Task.
func AttentionRank(failed, warned, running, pending bool) int {
	switch {
	case failed:
		return liveRank(core.TaskSnapshot{State: core.Failed})
	case warned:
		return 1
	case running:
		return liveRank(core.TaskSnapshot{State: core.Running})
	case pending:
		return liveRank(core.TaskSnapshot{State: core.Pending})
	default:
		return attentionRankCount
	}
}

// Keep adds an admitted child collection's live projection.
func (c *LiveCollections) Keep(col core.TasksSnapshot) { c.kept = append(c.kept, col) }

// Omit tallies a child collection the frame cannot reach from its Tasks'
// counts at any depth, and ownRow, the name of the one row it renders as
// when it renders as its own Task ("" otherwise).
func (c *LiveCollections) Omit(tasks core.ChildCounts, ownRow string) {
	c.tally.Count++
	c.tally.Tasks.Merge(tasks)
	if !tasks.Unfinished {
		c.tally.Settled++
	}
	if ownRow != "" {
		c.tally.OwnRows++
		c.tally.OwnRowNameWidth = max(c.tally.OwnRowNameWidth, len([]rune(ownRow)))
	}
}

// OwnRowName is the name col renders its one row under when it renders
// as its own Task, or "".
func OwnRowName(col core.TasksSnapshot) string {
	name, _ := render.OwnTaskRowName(col)
	return name
}

// Kept is the child collections the frame could reach, in declaration
// order.
func (c *LiveCollections) Kept() []core.TasksSnapshot { return c.kept }

// Tally is what the projection counted of the ones it left out.
func (c *LiveCollections) Tally() core.CollectionTally { return c.tally }

// Into is col holding the kept child collections and the tally of the
// rest.
func (c *LiveCollections) Into(col core.TasksSnapshot) core.TasksSnapshot {
	col.Collections = c.kept
	return core.WithCollectionTally(col, c.tally)
}
