package render

import (
	"slices"

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

// isDispositionItem reports whether t's only information is its Kept or
// Skipped record, so its row would say nothing its Group's tally does not.
func isDispositionItem(t core.TaskSnapshot) bool {
	if t.State != core.Done && t.State != core.Skipped || t.Synthetic() {
		return false
	}
	if len(t.Kept) == 0 && len(t.Skipped) == 0 {
		return false
	}
	return t.Summary == "" && t.Phase == "" && t.Progress.Total == 0 && t.Progress.Completed == 0 &&
		len(t.Problems) == 0 && len(t.Warnings) == 0 && len(t.Facts) == 0 &&
		len(t.Actions) == 0 && len(t.Verification) == 0
}

// withoutDispositionItems returns col without its disposition items, and
// their summed tallies — one pass over the children, in child order, with
// no sorting, so a live frame can afford it every tick. A Sequence keeps
// every row: its rows state an order the tally cannot (§5, and the
// zero-information rule's same exemption).
func withoutDispositionItems(col core.TasksSnapshot) (core.TasksSnapshot, core.Dispositions) {
	var items core.Dispositions
	if col.Sequential || !slices.ContainsFunc(col.Tasks, isDispositionItem) {
		return col, items
	}
	// No preallocation: a TaskSnapshot is large, and the rows that survive
	// are typically the one work Task, not the thousand items.
	var rest []core.TaskSnapshot
	for _, t := range col.Tasks {
		if isDispositionItem(t) {
			items.AddTask(t)
			continue
		}
		rest = append(rest, t)
	}
	col.Tasks = rest
	return col, items
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
