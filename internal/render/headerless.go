package render

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
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

// hasUnfinishedTask reports whether any Task at or below col is still
// pending or running. While work is in flight the live header carries the
// Group's own aggregate progress ("N/M complete", with its elapsed time), so
// it is information beyond the children and stays. Once every Task is
// terminal that count is spent and the header is dropped like the durable
// one.
func hasUnfinishedTask(col core.TasksSnapshot) bool {
	if slices.ContainsFunc(col.Tasks, func(t core.TaskSnapshot) bool { return !core.IsTerminalTask(t.State) }) {
		return true
	}
	return slices.ContainsFunc(col.Collections, hasUnfinishedTask)
}

// writeLiveHeaderlessGroup is writeHeaderlessGroup for the live region: each
// child paints as its own root-level row (spinner, bar, count, and the one
// indented activity row under it), aligned to one name column. Rows beyond
// the height budget collapse into one "not shown" line, as under a header.
func writeLiveHeaderlessGroup(b *strings.Builder, col core.TasksSnapshot, height, width int, spin string, color bool, now time.Time, profile txt.GlyphProfile) {
	nameWidth := maxRootTaskNameWidth(col.Tasks)
	selected, omitted := selectLiveChildren(col.Tasks, max(height-1, 1))
	for _, t := range selected {
		writeLiveTaskLine(b, t, 0, nameWidth, width, spin, color, now, profile)
	}
	if omitted > 0 {
		fmt.Fprintf(b, "%s  %d not shown\n", txt.Dim(txt.GlyphOverflow.Render(profile), color), omitted)
	}
	for _, child := range col.Collections {
		writeLiveCollection(b, child, height, width, spin, color, now, profile)
	}
}

// writeHeaderlessGroup renders a Group's children as siblings of the
// surrounding rows, aligned to one name column, then any nested containers
// the same way.
func writeHeaderlessGroup(b *strings.Builder, col core.TasksSnapshot, color, verbose bool, profile txt.GlyphProfile) {
	nameWidth := maxTaskNameWidth(col.Tasks)
	for _, t := range col.Tasks {
		WriteTaskAligned(b, t, nameWidth, color, verbose, profile)
	}
	for _, child := range col.Collections {
		WriteCollection(b, child, color, verbose, profile)
	}
}
