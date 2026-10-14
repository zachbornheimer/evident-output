package live

import (
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
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
	kept  []record.TaskSnapshot

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
	All, Work project.ChildCounts
	// Items sums the Excluded items' Excluded reasons.
	Items  project.Tally
	Census project.ChildCensus
}

// ChildKind is how a live frame files one child Task of the collection
// named group.
type ChildKind struct {
	// Item reports a Excluded item: counted in its collection's
	// tally, never ranked.
	Item bool
	// OwnTask feeds the census that decides whether the items fold
	// (project.ChildCensus).
	OwnTask bool
	// Rank is the attention rank of a non-item child; AttentionRanks or
	// more is routine.
	Rank int
}

// AttentionRanks is the rank of a child that never fills a frame's rows
// by attention.
const AttentionRanks = attentionRankCount

// ClassifyChild files t under the collection named group.
func ClassifyChild(group string, t *record.TaskSnapshot) ChildKind {
	switch {
	case project.IsOwnTask(group, t):
		return ChildKind{OwnTask: true, Rank: liveRank(*t)}
	case project.IsExcludedItem(group, t):
		return ChildKind{Item: true}
	default:
		return ChildKind{Rank: liveRank(*t)}
	}
}

// ChildRows is how many rows of children a frame of rows rows admits by
// attention rank; it admits one more, in declaration order, of each of
// the Excluded items and the other children.
func ChildRows(rows int) int { return liveHeight(rows) }

// NewLiveChildren starts the projection of the children of the collection
// named group for a frame of rows rows.
func NewLiveChildren(group string, rows int) *LiveChildren {
	return &LiveChildren{group: group, rows: liveHeight(rows)}
}

// Admit counts t and reports whether the frame could show it, in which
// case the caller passes its full snapshot to Keep.
func (c *LiveChildren) Admit(t *record.TaskSnapshot) bool {
	r := &c.roster
	r.All.Add(t)
	kind := ClassifyChild(c.group, t)
	switch {
	case kind.Item:
		r.Census.Items++
		r.Items.AddItem(t)
		c.keptItems++
		return c.keptItems <= c.rows+1
	case kind.OwnTask:
		r.Census.OwnTask = true
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
func (c *LiveChildren) Keep(t record.TaskSnapshot) { c.kept = append(c.kept, t) }

// Collection is col, whose own fields the caller filled, holding the kept
// children and, when some were left out, the tally of all of them.
func (c *LiveChildren) Collection(col record.TasksSnapshot) record.TasksSnapshot {
	return c.roster.Project(col, c.kept)
}

// Project is col holding kept, the children a frame could show, and,
// when some were left out, the tally of all of them.
func (r ChildRoster) Project(col record.TasksSnapshot, kept []record.TaskSnapshot) record.TasksSnapshot {
	col.Tasks = kept
	if len(kept) == r.All.Total {
		return project.WithoutChildTally(col)
	}
	tally := project.ChildTally{All: r.All, Rest: r.All}
	if !col.Sequential && r.Census.Folds() {
		tally.Folded, tally.Items, tally.Rest = true, r.Items, r.Work
	}
	return project.WithChildTally(col, tally)
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
	kept   []record.TasksSnapshot
	tally  project.CollectionTally
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
// any depth, include a failed, a running or a pending one: the
// most urgent that applies, as liveRank orders a single Task.
func AttentionRank(failed, running, pending bool) int {
	switch {
	case failed:
		return liveRank(record.TaskSnapshot{State: record.Failed})
	case running:
		return liveRank(record.TaskSnapshot{State: record.Running})
	case pending:
		return liveRank(record.TaskSnapshot{State: record.Pending})
	default:
		return attentionRankCount
	}
}

// Keep adds an admitted child collection's live projection.
func (c *LiveCollections) Keep(col record.TasksSnapshot) { c.kept = append(c.kept, col) }

// Omit tallies a child collection the frame cannot reach from its Tasks'
// counts at any depth, and ownRow, the name of the one row it renders as
// when it renders as its own Task ("" otherwise).
func (c *LiveCollections) Omit(tasks project.ChildCounts, ownRow string) {
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
func OwnRowName(col record.TasksSnapshot) string {
	name, _ := project.OwnTaskRowName(col)
	return name
}

// Kept is the child collections the frame could reach, in declaration
// order.
func (c *LiveCollections) Kept() []record.TasksSnapshot { return c.kept }

// Tally is what the projection counted of the ones it left out.
func (c *LiveCollections) Tally() project.CollectionTally { return c.tally }

// Into is col holding the kept child collections and the tally of the
// rest.
func (c *LiveCollections) Into(col record.TasksSnapshot) record.TasksSnapshot {
	col.Collections = c.kept
	return project.WithCollectionTally(col, c.tally)
}
