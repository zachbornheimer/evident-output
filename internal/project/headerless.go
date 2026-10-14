package project

import (
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// groupHeaderAddsNothing reports whether a Group's own row would say
// nothing its children do not already say (contract §3, §13: Group and
// Sequence organize Tasks; the renderer decides whether the container label
// deserves a row). A Group owns no Problem, Fact, Progress or Effect of its
// own; the only information it can carry is a caller-set Summary. Without
// one, its state is derived from its children and its name is only a
// category label, so the header is dropped and the children render as
// siblings. A Sequence keeps its header because its order is meaning the
// children alone do not state. A Group populated by Build keeps its header
// too: its items are discovered while the run goes, and the header is the
// row they belong to. Machine output never consults this: the Group stays
// in the snapshot and the JSON document.
func groupHeaderAddsNothing(col record.TasksSnapshot) bool {
	return !col.Sequential && !col.Built && col.Summary == ""
}

// FlattensHeader reports whether col, with items already folded out of it,
// renders without its header row: its children then render as siblings of
// the rows around it. Folded tallies need the header to hang under.
func FlattensHeader(col record.TasksSnapshot, items Dispositions) bool {
	return groupHeaderAddsNothing(col) && items.Empty()
}

// HasUnfinishedTask reports whether any Task at or below col is still
// pending or running. While work is in flight the live header carries the
// Group's own aggregate progress ("N/M complete", with its elapsed time), so
// it is information beyond the children and stays. Once every Task is
// terminal that count is spent and the header is dropped like the durable
// one.
func HasUnfinishedTask(col record.TasksSnapshot) bool {
	if OwnCounts(col).Unfinished || CollectionTallyOf(col).Tasks.Unfinished {
		return true
	}
	return slices.ContainsFunc(col.Collections, HasUnfinishedTask)
}

// LiveProgressAddsInformation reports whether a live Group header's
// "N/M complete" says something its rows do not: it counts the Group's own
// child Tasks (total of them), so it informs only while one of them, or
// work below them, is unfinished. A Group that holds only nested Groups
// counts nothing — each nested Group paints its own progress — so its
// header would read "0/0 complete" for the whole run (zq prune's
// "categories" Group of category Groups, contract §18's live frame).
func LiveProgressAddsInformation(col record.TasksSnapshot, total int) bool {
	return total > 0 && HasUnfinishedTask(col)
}

// OwnCounts summarizes col's own child Tasks, left-out ones included.
func OwnCounts(col record.TasksSnapshot) ChildCounts {
	if tally, ok := ChildTallyOf(col); ok {
		return tally.All
	}
	return CountTasks(col.Tasks)
}
