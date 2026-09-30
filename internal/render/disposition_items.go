package render

import (
	"github.com/zachbornheimer/evident-output/internal/core"
)

// A Group's per-item children that did nothing but resolve Kept or Skipped
// (the item is the Task — docs/reference.md) are counted, not listed:
// "Rendering every child is not a correctness requirement; retaining every
// child in the model is" and "aggregation is a renderer concern" (contract
// §25). Human output folds them into one tally per disposition under the
// Group's row ("  ! kept N (...)", §26/§27), and --verbose names every
// item under its reason. Machine output never calls this; JSON and JSONL
// keep every child Task.

// isDispositionItem reports whether t is an item of the Group named group
// whose only information is its Kept or Skipped record, so its row would
// say nothing its Group's tally does not. The Group's own Task is never an item: it is
// the row the tally hangs under.
func isDispositionItem(group string, t *core.TaskSnapshot) bool {
	if t.State != core.Done && t.State != core.Skipped || t.Synthetic() || isOwnTask(group, t) {
		return false
	}
	if len(t.Kept) == 0 && len(t.Skipped) == 0 {
		return false
	}
	// Facts ride the item into its tally (verbose lists them beside the
	// item's name), so they never break the fold (E-100).
	return t.Summary == "" && t.Phase == "" && t.Progress.Total == 0 && t.Progress.Completed == 0 &&
		len(t.Problems) == 0 && len(t.Warnings) == 0 &&
		len(t.Actions) == 0 && len(t.Verification) == 0
}

// minFoldedItems is the fewest disposition items a tally replaces when
// no own Task names their subject. One item's own row already is its
// count, and it carries the item's name: a lone Skipped peer
// ("○ remote-tracking  - skipped 1 (--skip-fetch)") must not become a
// nameless "- skipped 1" under a header its Group never had. Under a
// Group's own Task every item is an item of that subject, so one is
// enough ("✓ branches  2 checked" / "  ! kept 1 (protected)").
const (
	minFoldedItems        = 2
	minFoldedItemsOwnTask = 1
)

// childCensus is how a Group's child Tasks partition for folding: its own
// Task, its disposition items, and whether any other child finished work
// of its own (a work peer).
type childCensus struct {
	items    int
	ownTask  bool
	workPeer bool
}

func censusOf(col core.TasksSnapshot) childCensus {
	var c childCensus
	for i := range col.Tasks {
		t := &col.Tasks[i]
		switch {
		case isOwnTask(col.Name, t):
			c.ownTask = true
		case isDispositionItem(col.Name, t):
			c.items++
		case isWorkPeer(t):
			c.workPeer = true
		}
	}
	return c
}

// isWorkPeer reports whether t, a child that is neither its Group's own
// Task nor a disposition item, finished work of its own worth a row. Its
// presence says the Group's children are peer subjects (categories), not
// items of one subject. A child still in flight is not one yet: it may
// still resolve as an item.
func isWorkPeer(t *core.TaskSnapshot) bool {
	return core.IsTerminalTask(t.State) && !core.IsZeroInformationTask(*t)
}

// foldsItems reports whether col's disposition items fold into a tally:
// any of them under a Group's own Task, else at least minFoldedItems
// under a Group whose Summary names their subject or that has no work
// peer. Beside a work peer, with no such row, a Skipped or
// Kept child is a peer category whose name is its information ("○ tags
// - skipped 1 (--skip-fetch)"), so it keeps its row. A Sequence keeps
// every row: its rows state an order the tally cannot (§5, and the
// zero-information rule's same exemption).
func foldsItems(col core.TasksSnapshot) bool {
	if col.Sequential {
		return false
	}
	if tally, ok := core.ChildTallyOf(col); ok {
		return tally.Folded
	}
	return censusOf(col).folds(col.Summary)
}

// folds is foldsItems' rule for a Group with this census and summary.
func (c childCensus) folds(summary string) bool {
	if c.ownTask {
		return c.items >= minFoldedItemsOwnTask
	}
	return c.items >= minFoldedItems && (summary != "" || !c.workPeer)
}

// withoutDispositionItems returns col without its disposition items, and
// their summed tallies, when it folds them (foldsItems) — linear in the
// children, in child order, with no sorting, so a live frame can afford
// it every tick.
func withoutDispositionItems(col core.TasksSnapshot) (core.TasksSnapshot, core.Dispositions) {
	var items core.Dispositions
	if !foldsItems(col) {
		return col, items
	}
	// No preallocation: a TaskSnapshot is large, and the rows that survive
	// are typically the one work Task, not the thousand items.
	tally, partial := core.ChildTallyOf(col)
	var rest []core.TaskSnapshot
	for i := range col.Tasks {
		t := &col.Tasks[i]
		if isDispositionItem(col.Name, t) {
			items.AddTask(t)
			continue
		}
		rest = append(rest, *t)
	}
	col.Tasks = rest
	if !partial {
		return col, items
	}
	// A live projection kept only some items; its tally summed them all.
	if len(rest) == tally.Rest.Total {
		return core.WithoutChildTally(col), tally.Items
	}
	return core.WithChildTally(col, core.ChildTally{All: tally.Rest, Rest: tally.Rest}), tally.Items
}

// headerTallyIndent is where a Group header's folded tallies start. Beside
// surviving child rows they are the header's children too and share the
// child column; with no child row left they annotate the header itself,
// as a Task's tallies annotate its row ("✓ branches  6 checked" /
// "  ! kept 3 (...)", §26/§27).
func headerTallyIndent(folded core.TasksSnapshot) string {
	if len(folded.Tasks) > 0 || len(folded.Collections) > 0 {
		return groupChildIndent
	}
	return taskAnnotationIndent
}

// headerlessRowNameWidth is the shared name column of a header-less
// Group's rows: its own Tasks plus every child collection that collapses
// into one row. Zero when fewer than two rows share it.
func headerlessRowNameWidth(col core.TasksSnapshot) int {
	rows, width := len(col.Tasks), 0
	for _, t := range col.Tasks {
		width = max(width, len([]rune(t.Name)))
	}
	// A projection's left-out rows still set the column, as they do in
	// the whole collection.
	if tally, ok := core.ChildTallyOf(col); ok {
		rows, width = tally.All.Total, max(width, tally.All.NameWidth)
	}
	for _, child := range col.Collections {
		if name, ok := ownTaskRowName(child); ok {
			rows++
			width = max(width, len([]rune(name)))
		}
	}
	left := core.CollectionTallyOf(col)
	rows, width = rows+left.OwnRows, max(width, left.OwnRowNameWidth)
	if rows < 2 {
		return 0
	}
	return width
}
