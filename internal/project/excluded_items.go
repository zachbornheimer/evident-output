package project

import (
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A Group's per-item children that did nothing but resolve Excluded (the
// item is the Task, its one Problem the Exclude reason) are counted, not listed:
// rendering every child is not a correctness requirement, retaining every
// child in the model is, and aggregation is a renderer concern (CONTRACT §8:
// Excluded Tasks fold into one tally). Human output folds them into one
// tally under the Group's row ("  ○ excluded N (...)"), and --verbose names
// every item under its reason. Machine output never calls this; JSON and
// JSONL keep every child Task.

// IsExcludedItem reports whether t is an item of the Group named group
// whose only information is that it was Excluded for a reason, so its row
// would say nothing its Group's tally does not. The Group's own Task is
// never an item: it is the row the tally hangs under.
func IsExcludedItem(group string, t *record.TaskSnapshot) bool {
	if t.Synthetic() || IsOwnTask(group, t) {
		return false
	}
	reason, ok := ExcludeReason(t)
	// Facts ride the item into its tally (verbose lists them beside the
	// item's name), so they never break the fold (E-100).
	return ok && (t.Summary == "" || t.Summary == reason) && t.Phase == "" && t.Progress.Total == 0 && t.Progress.Completed == 0 &&
		len(t.Actions) == 0 && len(t.Verification) == 0
}

// ExcludeReason is why t was Excluded: the Summary of the one Problem that
// Exclude recorded. It reports false for any other Task, and for an
// Excluded Task whose Problem says more than the reason (a Detail, subject,
// Next action or other structure), since that row has information its
// tally would drop.
func ExcludeReason(t *record.TaskSnapshot) (string, bool) {
	if t.State != record.Excluded || len(t.Problems) != 1 {
		return "", false
	}
	p := &t.Problems[0]
	bare := p.Summary != "" && p.Subject == "" && p.Detail == "" && p.EvidenceTail == "" && p.Count == 0 &&
		p.Location == nil && p.Cause == nil && len(p.Evidence) == 0 && len(p.Actions) == 0 && len(p.Fields) == 0
	return p.Summary, bare
}

// minFoldedItems is the fewest Excluded items a tally replaces when no own
// Task hosts it. One item's own row already is its count, and it carries the
// item's name. Under a Group's own Task every item is an item of that
// subject, so one is enough ("✓ branches  2 checked" / "  ○ excluded 1
// (protected)").
const (
	minFoldedItems        = 2
	minFoldedItemsOwnTask = 1
)

// ChildCensus is how a Group's child Tasks partition for folding: its own
// Task and its Excluded items.
type ChildCensus struct {
	Items   int
	OwnTask bool
}

func censusOf(col record.TasksSnapshot) ChildCensus {
	var c ChildCensus
	for i := range col.Tasks {
		t := &col.Tasks[i]
		switch {
		case IsOwnTask(col.Name, t):
			c.OwnTask = true
		case IsExcludedItem(col.Name, t):
			c.Items++
		}
	}
	return c
}

// foldsItems reports whether col's Excluded items fold into a tally:
// two or more of them, or any under a Group's own Task. A Sequence keeps
// every row: its rows state an order the tally cannot (§5, and the
// zero-information rule's same exemption).
func foldsItems(col record.TasksSnapshot) bool {
	if col.Sequential {
		return false
	}
	if tally, ok := ChildTallyOf(col); ok {
		return tally.Folded
	}
	return censusOf(col).Folds()
}

// Folds is foldsItems' rule for a Group with this census.
func (c ChildCensus) Folds() bool {
	if c.OwnTask {
		return c.Items >= minFoldedItemsOwnTask
	}
	return c.Items >= minFoldedItems
}

// WithoutExcludedItems returns col without its Excluded items, and
// their summed tallies, when it folds them (foldsItems) — linear in the
// children, in child order, with no sorting, so a live frame can afford
// it every tick.
func WithoutExcludedItems(col record.TasksSnapshot) (record.TasksSnapshot, Tally) {
	var items Tally
	if !foldsItems(col) {
		return col, items
	}
	// No preallocation: a TaskSnapshot is large, and the rows that survive
	// are typically the one work Task, not the thousand items.
	tally, partial := ChildTallyOf(col)
	var rest []record.TaskSnapshot
	for i := range col.Tasks {
		t := &col.Tasks[i]
		if IsExcludedItem(col.Name, t) {
			items.AddItem(t)
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
		return WithoutChildTally(col), tally.Items
	}
	return WithChildTally(col, ChildTally{All: tally.Rest, Rest: tally.Rest}), tally.Items
}

// HeaderTallyIndent is where a Group header's folded tallies start. Beside
// surviving child rows they are the header's children too and share the
// child column; with no child row left they annotate the header itself,
// as a Task's tallies annotate its row ("✓ branches  6 checked" /
// "  ○ excluded 3 (...)", §26/§27).
func HeaderTallyIndent(folded record.TasksSnapshot) string {
	if len(folded.Tasks) > 0 || len(folded.Collections) > 0 {
		return GroupChildIndent
	}
	return TaskAnnotationIndent
}

// HeaderlessRowNameWidth is the shared name column of a header-less
// Group's rows: its own Tasks plus every child collection that collapses
// into one row. Zero when fewer than two rows share it.
func HeaderlessRowNameWidth(col record.TasksSnapshot) int {
	rows, width := len(col.Tasks), 0
	for _, t := range col.Tasks {
		width = max(width, len([]rune(t.Name)))
	}
	// A projection's left-out rows still set the column, as they do in
	// the whole collection.
	if tally, ok := ChildTallyOf(col); ok {
		rows, width = tally.All.Total, max(width, tally.All.NameWidth)
	}
	for _, child := range col.Collections {
		if name, ok := OwnTaskRowName(child); ok {
			rows++
			width = max(width, len([]rune(name)))
		}
	}
	left := CollectionTallyOf(col)
	rows, width = rows+left.OwnRows, max(width, left.OwnRowNameWidth)
	if rows < 2 {
		return 0
	}
	return width
}
