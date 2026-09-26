package render

import (
	"github.com/zachbornheimer/evident-output/internal/core"
)

// groupHeaderAddsNothing reports whether a Group's own row would say
// nothing its children do not already say (contract §3, §13: Group and
// Sequence organize Tasks; the renderer decides whether the container label
// deserves a row). A Group owns no Problem, Fact, Progress or Effect of its
// own; the only information it can carry is a caller-set Summary. Without
// one, its state is derived from its children and its name is only a
// category label, so the header is dropped and the children render as
// siblings. A Sequence keeps its header because its order is meaning the
// children alone do not state. Machine output never consults this: the
// Group stays in the snapshot and the JSON document.
func groupHeaderAddsNothing(col core.TasksSnapshot) bool {
	return !col.Sequential && col.Summary == ""
}

// FlattensHeader reports whether col, with items already folded out of it,
// renders without its header row: its children then render as siblings of
// the rows around it. Folded tallies need the header to hang under.
func FlattensHeader(col core.TasksSnapshot, items core.Dispositions) bool {
	return groupHeaderAddsNothing(col) && items.Empty()
}
