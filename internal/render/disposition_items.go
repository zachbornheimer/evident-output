package render

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
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
	return t.Summary == "" && t.Phase == "" && t.Progress.Total == 0 && t.Progress.Completed == 0 &&
		len(t.Problems) == 0 && len(t.Warnings) == 0 && len(t.Facts) == 0 &&
		len(t.Actions) == 0 && len(t.Verification) == 0
}

// minFoldedItems is the fewest disposition items a tally replaces. One
// item's own row already is its count, and it carries the item's name: a
// lone Skipped peer ("○ remote-tracking  - skipped 1 (--skip-fetch)")
// must not become a nameless "- skipped 1" under a header its Group never
// had.
const minFoldedItems = 2

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
	return core.IsTerminalTask(t.State) && !IsZeroInformationTask(*t)
}

// foldsItems reports whether col's disposition items fold into a tally:
// at least minFoldedItems of them, under a Group whose own row names the
// subject they are items of — its own Task or its own Summary — or that
// has no work peer. Beside a work peer, with no such row, a Skipped or
// Kept child is a peer category whose name is its information ("○ tags
// - skipped 1 (--skip-fetch)"), so it keeps its row. A Sequence keeps
// every row: its rows state an order the tally cannot (§5, and the
// zero-information rule's same exemption).
func foldsItems(col core.TasksSnapshot) bool {
	if col.Sequential {
		return false
	}
	c := censusOf(col)
	namesSubject := c.ownTask || col.Summary != ""
	return c.items >= minFoldedItems && (namesSubject || !c.workPeer)
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
	return col, items
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

// writeLiveDispositions writes items' tallies at indent as the live frame
// shows them (never verbose) within maxRows, and reports how many rows they took, so
// the frame's height budget can count them. When the cause lines do not
// fit, each tally keeps its headline and drops its causes: the headline is
// the count, the durable render still carries the evidence.
func writeLiveDispositions(b *strings.Builder, indent string, items core.Dispositions, maxRows int, color bool, profile txt.GlyphProfile) (rows int) {
	if items.Empty() {
		return 0
	}
	var full strings.Builder
	writeDispositions(&full, indent, items, "", false, color, profile)
	if rows = strings.Count(full.String(), "\n"); rows <= maxRows {
		b.WriteString(full.String())
		return rows
	}
	start := b.Len()
	writeTaxonomyHeadline(b, indent, taxonomySkipped, items.Skipped, color, profile)
	writeTaxonomyHeadline(b, indent, taxonomyKept, items.Kept, color, profile)
	return strings.Count(b.String()[start:], "\n")
}

// headerlessRowNameWidth is the shared name column of a header-less
// Group's rows: its own Tasks plus every child collection that collapses
// into one row. Zero when fewer than two rows share it.
func headerlessRowNameWidth(col core.TasksSnapshot) int {
	rows, width := len(col.Tasks), 0
	for _, t := range col.Tasks {
		width = max(width, len([]rune(t.Name)))
	}
	for _, child := range col.Collections {
		if name, ok := ownTaskRowName(child); ok {
			rows++
			width = max(width, len([]rune(name)))
		}
	}
	if rows < 2 {
		return 0
	}
	return width
}
