package plain

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/render"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Plain projects a snapshot to plain text without terminal ownership.
// width <= 0 falls back to defaultWidth.
func Plain(s core.Snapshot, width int, noColor, verbose bool, profile txt.GlyphProfile) string {
	var b strings.Builder
	if width <= 0 {
		width = render.DefaultWidth
	}
	st := render.Style{Color: !noColor, Verbose: verbose, Profile: profile}
	s = render.HumanProjection(s, verbose)

	if s.DryRun {
		WritePlannedHeader(&b, st.Color, s.Preview, s.DryRunSubject)
	}

	for _, line := range s.Lines {
		WriteDebugOrLine(&b, line, st.Color)
	}
	render.WriteRunAnnotations(&b, s.Warnings, s.Facts, st)

	taskNameWidth := maxTaskNameWidth(s.Tasks)
	for _, t := range s.Tasks {
		render.WriteTaskAligned(&b, t, taskNameWidth, st)
	}

	for _, col := range s.Collections {
		WriteCollection(&b, col, st)
	}

	if render.HasTaskRows(s) && render.HasEffectSections(s) {
		b.WriteByte('\n')
	}

	render.WriteLedger(&b, s, width, st)

	if s.Conclusion != nil && !render.ShouldSuppressStandaloneConclusion(s) {
		WriteConclusion(&b, render.StandaloneConclusion(s), st)
	}

	return b.String()
}

// maxTaskNameWidth returns the shared column sibling root tasks pad their
// name to (fixture-repo-retire-dryrun.md) — the widest name's cell width. A
// lone task (or none) needs no alignment, so callers pass the result
// straight to render.WriteTaskAligned's nameWidth, where 0 means "don't pad".
func maxTaskNameWidth(tasks []core.TaskSnapshot) int {
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
		b.WriteString(txt.Dim(line, color))
		b.WriteByte('\n')
		return
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

// WriteCollection renders a Tasks group: the parent glyph/name (with its own
// Summary when set), then every resolved child row with its own summary or
// problem — core.Done included. Evo-rec.md core.Problem 1's final ledger keeps ✓ rows
// like "✓  branches   14 deleted" instead of the parent collapsing to one
// line and erasing the children whose evidence lived only in the live
// region while it was running.
func WriteCollection(b *strings.Builder, col core.TasksSnapshot, s render.Style) {
	writeCollectionAligned(b, col, 0, s)
}

// writeCollectionAligned is WriteCollection with the name column a
// collapsed one-row collection pads to (0 = its own name), so a header-less
// parent's rows line up (headerlessRowNameWidth).
func writeCollectionAligned(b *strings.Builder, col core.TasksSnapshot, nameWidth int, s render.Style) {
	col, items := render.WithoutDispositionItems(col)
	switch {
	case render.RendersAsOwnTask(col):
		render.WriteTaskAligned(b, col.Tasks[0], nameWidth, s)
		render.WriteDispositions(b, render.TaskAnnotationIndent, items, render.NoDisposition, s)
	case render.FlattensHeader(col, items):
		writeHeaderlessGroup(b, col, s)
	default:
		writeCollectionHeader(b, col, s)
		render.WriteDispositions(b, render.HeaderTallyIndent(col), items, render.NoDisposition, s)
		writeCollectionBody(b, col, s)
	}
}

// writeCollectionHeader writes a Group or Sequence's own row.
func writeCollectionHeader(b *strings.Builder, col core.TasksSnapshot, s render.Style) {
	unit := render.DisplayUnit{Glyph: s.StateGlyph(col.State), Name: col.Name}
	if col.Summary != "" {
		unit.Detail = s.Dim(col.Summary)
	}
	b.WriteString(unit.Render(""))
	b.WriteByte('\n')
}

// writeCollectionBody writes a headed container's child rows, then its
// nested containers indented one level per nesting depth (P3).
func writeCollectionBody(b *strings.Builder, col core.TasksSnapshot, s render.Style) {
	childNameWidth := maxTaskNameWidth(col.Tasks)
	for _, t := range col.Tasks {
		render.ChildRow(t, childNameWidth).Write(b, s)
	}
	for _, child := range col.Collections {
		var nested strings.Builder
		WriteCollection(&nested, child, s)
		for line := range strings.SplitSeq(strings.TrimRight(nested.String(), "\n"), "\n") {
			fmt.Fprintf(b, "%s%s\n", render.GroupChildIndent, line)
		}
	}
}
