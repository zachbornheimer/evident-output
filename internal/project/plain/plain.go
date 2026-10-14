package plain

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Plain projects a snapshot to plain text without terminal ownership.
// width <= 0 falls back to defaultWidth.
func Plain(s record.Snapshot, width int, noColor, verbose bool, profile project.GlyphProfile) string {
	var b strings.Builder
	if width <= 0 {
		width = project.DefaultWidth
	}
	st := project.Style{Color: !noColor, Verbose: verbose, Profile: profile}
	s = project.HumanProjection(s, verbose)

	if s.DryRun {
		WritePlannedHeader(&b, st.Color)
	}

	for _, line := range s.Lines {
		WriteDebugOrLine(&b, line, st.Color)
	}
	project.WriteRunAnnotations(&b, s.Warnings, s.Facts, st)

	taskNameWidth := maxTaskNameWidth(s.Tasks)
	for _, t := range s.Tasks {
		project.WriteTaskAligned(&b, t, taskNameWidth, st)
	}

	for _, col := range s.Collections {
		WriteCollection(&b, col, st)
	}

	if project.HasTaskRows(s) && project.HasEffectSections(s) {
		b.WriteByte('\n')
	}

	project.WriteLedger(&b, s, width, st)

	if s.Conclusion != nil && !project.ShouldSuppressStandaloneConclusion(s) {
		WriteConclusion(&b, *s.Conclusion, st)
	}

	return b.String()
}

// maxTaskNameWidth returns the shared column sibling root tasks pad their
// name to (fixture-repo-retire-dryrun.md) — the widest name's cell width. A
// lone task (or none) needs no alignment, so callers pass the result
// straight to project.WriteTaskAligned's nameWidth, where 0 means "don't pad".
func maxTaskNameWidth(tasks []record.TaskSnapshot) int {
	if len(tasks) < 2 {
		return 0
	}
	width := 0
	for _, t := range tasks {
		if n := len([]rune(t.Name)); n > width {
			width = n
		}
	}
	return width
}

// WriteDebugOrLine formats a stored line; dims history/pane debug grammar
// when color is on. Shared by Plain and the root package's own residual
// composition (progressive.go), which also renders stored lines.
func WriteDebugOrLine(b *strings.Builder, line string, color bool) {
	if strings.Contains(line, "[DEBUG]") || strings.Contains(line, " level=DEBUG ") {
		b.WriteString(project.Dim(line, color))
		b.WriteByte('\n')
		return
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

// WriteCollection renders a Tasks group: the parent glyph/name (with its own
// Summary when set), then every resolved child row with its own summary or
// problem — Succeeded included. Evo-rec.md record.Problem 1's final ledger keeps ✓ rows
// like "✓  branches   14 deleted" instead of the parent collapsing to one
// line and erasing the children whose evidence lived only in the live
// region while it was running.
func WriteCollection(b *strings.Builder, col record.TasksSnapshot, s project.Style) {
	writeCollectionAligned(b, col, 0, s)
}

// writeCollectionAligned is WriteCollection with the name column a
// collapsed one-row collection pads to (0 = its own name).
func writeCollectionAligned(b *strings.Builder, col record.TasksSnapshot, nameWidth int, s project.Style) {
	col, items := project.WithoutDispositionItems(col)
	switch {
	case project.RendersAsOwnTask(col):
		project.WriteTaskAligned(b, col.Tasks[0], nameWidth, s)
		project.WriteDispositions(b, project.TaskAnnotationIndent, items, project.NoDisposition, s)
	default:
		writeCollectionHeader(b, col, s)
		project.WriteDispositions(b, project.HeaderTallyIndent(col), items, project.NoDisposition, s)
		writeCollectionBody(b, col, s)
	}
}

// writeCollectionHeader writes a Group or Sequence's own row.
func writeCollectionHeader(b *strings.Builder, col record.TasksSnapshot, s project.Style) {
	unit := project.DisplayUnit{Glyph: s.StateGlyph(col.State), Name: col.Name}
	if col.Summary != "" {
		unit.Detail = s.Dim(col.Summary)
	}
	b.WriteString(unit.Render(""))
	b.WriteByte('\n')
}

// writeCollectionBody writes a headed container's child rows, then its
// nested containers indented one level per nesting depth (P3).
func writeCollectionBody(b *strings.Builder, col record.TasksSnapshot, s project.Style) {
	childNameWidth := maxTaskNameWidth(col.Tasks)
	for _, t := range col.Tasks {
		project.ChildRow(t, childNameWidth).Write(b, s)
	}
	for _, child := range col.Collections {
		var nested strings.Builder
		WriteCollection(&nested, child, s)
		for line := range strings.SplitSeq(strings.TrimRight(nested.String(), "\n"), "\n") {
			fmt.Fprintf(b, "%s%s\n", project.GroupChildIndent, line)
		}
	}
}
