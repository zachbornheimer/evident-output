package render

import "github.com/zachbornheimer/evident-output/internal/core"

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

	all, work core.ChildCounts
	items     core.Dispositions
	census    childCensus
}

// NewLiveChildren starts the projection of the children of the collection
// named group for a frame of rows rows.
func NewLiveChildren(group string, rows int) *LiveChildren {
	return &LiveChildren{group: group, rows: liveHeight(rows)}
}

// Admit counts t and reports whether the frame could show it, in which
// case the caller passes its full snapshot to Keep.
func (c *LiveChildren) Admit(t *core.TaskSnapshot) bool {
	c.all.Add(t)
	switch {
	case isOwnTask(c.group, t):
		c.census.ownTask = true
	case isDispositionItem(c.group, t):
		c.census.items++
		c.items.AddTask(t)
		c.keptItems++
		return c.keptItems <= c.rows+1
	case isWorkPeer(t):
		c.census.workPeer = true
	}
	c.work.Add(t)
	c.keptWork++
	admit := c.keptWork <= c.rows+1
	if r := liveRank(*t); r < attentionRankCount {
		c.keptByRank[r]++
		admit = admit || c.keptByRank[r] <= c.rows
	}
	return admit
}

// Keep adds an admitted child's full snapshot.
func (c *LiveChildren) Keep(t core.TaskSnapshot) { c.kept = append(c.kept, t) }

// Collection is col, whose own fields the caller filled, holding the kept
// children and, when some were left out, the tally of all of them.
func (c *LiveChildren) Collection(col core.TasksSnapshot) core.TasksSnapshot {
	col.Tasks = c.kept
	if len(c.kept) == c.all.Total {
		return core.WithoutChildTally(col)
	}
	tally := core.ChildTally{All: c.all, Rest: c.all}
	if !col.Sequential && c.census.folds(col.Summary) {
		tally.Folded, tally.Items, tally.Rest = true, c.items, c.work
	}
	return core.WithChildTally(col, tally)
}

// ownCounts summarizes col's own child Tasks, left-out ones included.
func ownCounts(col core.TasksSnapshot) core.ChildCounts {
	if tally, ok := core.ChildTallyOf(col); ok {
		return tally.All
	}
	return core.CountTasks(col.Tasks)
}

// LiveCollections projects one collection's child collections for a live
// frame of at most rows rows. A frame paints nested collections in
// declaration order and examines at most one more of them than the rows
// it has left (liveFill.groups), so only the first rows+1 can ever
// appear: the projection keeps those, each projected in turn, and tallies
// the rest from a cheap view. Building a frame then costs what the screen
// shows, however many per-item collections the run holds (E-091).
type LiveCollections struct {
	rows  int
	kept  []core.TasksSnapshot
	tally core.CollectionTally
}

// NewLiveCollections starts the projection of a collection's child
// collections for a frame of rows rows.
func NewLiveCollections(rows int) *LiveCollections {
	return &LiveCollections{rows: liveHeight(rows)}
}

// Admit reports whether the frame could reach the next child collection,
// in declaration order. The caller passes an admitted one's live
// projection to Keep and any other's view to Omit.
func (c *LiveCollections) Admit() bool { return len(c.kept) <= c.rows }

// Keep adds an admitted child collection's live projection.
func (c *LiveCollections) Keep(col core.TasksSnapshot) { c.kept = append(c.kept, col) }

// Omit tallies a child collection the frame cannot reach. col need hold
// only its header and its Tasks' views, recursively.
func (c *LiveCollections) Omit(col core.TasksSnapshot) {
	c.tally.Count++
	countSubtree(&c.tally.Tasks, col)
	if name, ok := ownTaskRowName(col); ok {
		c.tally.OwnRows++
		c.tally.OwnRowNameWidth = max(c.tally.OwnRowNameWidth, len([]rune(name)))
	}
}

// countSubtree adds every Task at or below col to counts.
func countSubtree(counts *core.ChildCounts, col core.TasksSnapshot) {
	for i := range col.Tasks {
		counts.Add(&col.Tasks[i])
	}
	for _, child := range col.Collections {
		countSubtree(counts, child)
	}
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
