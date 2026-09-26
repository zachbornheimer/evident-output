// Package plain projects a core.Snapshot to durable, non-TTY text — the
// counterpart to render/live's interactive frames. It imports render for
// the shared row vocabulary; render never imports plain (see render/live's
// package doc for the mirrored rule on the live side).
package plain

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Render projects a snapshot to plain text without terminal ownership.
// width <= 0 falls back to render.DefaultWidth.
func Render(s core.Snapshot, width int, noColor, verbose bool, profile txt.GlyphProfile) string {
	var b strings.Builder
	if width <= 0 {
		width = render.DefaultWidth
	}
	st := render.Style{Color: !noColor, Verbose: verbose, Profile: profile}
	s = render.HumanProjection(s, verbose)

	if s.DryRun {
		render.WritePlannedHeader(&b, st.Color, s.Preview, s.DryRunSubject)
	}

	for _, line := range s.Lines {
		render.WriteDebugOrLine(&b, line, st.Color)
	}
	writeRunAnnotations(&b, s.Warnings, s.Facts, st)

	taskNameWidth := render.MaxTaskNameWidth(s.Tasks)
	for _, t := range s.Tasks {
		render.WriteTaskAligned(&b, t, taskNameWidth, st)
	}

	for _, col := range s.Collections {
		render.WriteCollection(&b, col, st)
	}

	if render.HasTaskRows(s) && render.HasEffectSections(s) {
		b.WriteByte('\n')
	}

	render.WriteLedger(&b, s, width, st)

	if s.Conclusion != nil && !render.ShouldSuppressStandaloneConclusion(s) {
		render.WriteConclusion(&b, render.StandaloneConclusion(s), st)
	}

	return b.String()
}

// writeRunAnnotations renders evo.Problem's warning-severity results and
// evo.Fact's run-scoped annotations (P8 symmetry with a task's own
// Problem/Fact) — fire-and-forget durable dim
// lines, warnings first: "! <text>" then "<name>  <value>", in call order
// within each severity.
func writeRunAnnotations(b *strings.Builder, warnings []core.Problem, facts []core.Fact, s render.Style) {
	glyph := s.WarningGlyph()
	for _, w := range warnings {
		fmt.Fprintf(b, "%s %s\n", glyph, render.WarningText(w))
	}
	for _, f := range facts {
		fmt.Fprintf(b, "%s\n", s.Dim(render.FactText(f)))
	}
}
