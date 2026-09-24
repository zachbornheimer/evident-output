package render

import (
	"fmt"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// inlineTaskWarning reports the one warning that belongs directly on t's own
// row (P2: "one short warning inlines on the ✓ row") — only when t has
// exactly one warning, it is short enough, and the row isn't already
// carrying a Summary of its own. Everything else nests below the row.
func inlineTaskWarning(t core.TaskSnapshot) (string, bool) {
	if t.State != core.Done || t.Summary != "" || len(t.Warnings) != 1 {
		return "", false
	}
	msg := t.Warnings[0].Summary
	if txt.Cells(msg) > warningInlineMaxCells {
		return "", false
	}
	return msg, true
}

// inlineWarningText renders an inline warning with the same "! " bang the
// nested writeNestedTaskWarnings line uses (E2.5 finding 3): the normative
// repo-retire dry-run fixture inlines a warning as "! kept 13 (...)" — an
// inline and a nested warning must signal identically, never a dim-only
// inline row that drops the one glyph the fixture treats as load-bearing.
func inlineWarningText(msg string, color bool, profile txt.GlyphProfile) string {
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
	return txt.Dim(glyph+" "+msg, color)
}

// inlineTaskTaxonomy mirrors inlineTaskWarning/inlineTaskFact for a task's
// accumulated Kept/Skipped disposition records (fixture-repo-retire-dryrun.md:
// "✓ branches          ! kept 13 (8 protected, 5 unpushed)" — a taxonomy
// tally IS a warning in the unified annotation model, so it competes for the
// same one-inline-annotation-per-row slot and is disqualified by the same
// conditions: a Summary, an explicit Warn, or an explicit Fact already claims
// the row. Both dispositions accumulated at once still nest below (rare, and
// two summaries cannot share one inline slot). The returned verb tells the
// caller which of Skipped/Kept was inlined, so its causes/Verbose name list
// (writeTaxonomy's other output) still renders below the row — inlining
// only replaces the headline count line, never the evidence under it.
func inlineTaskTaxonomy(t core.TaskSnapshot) (text string, verb disposition, ok bool) {
	if t.State != core.Done || t.Summary != "" || len(t.Warnings) != 0 || len(t.Facts) != 0 {
		return "", noDisposition, false
	}
	var records []core.TaxonomyRecord
	switch {
	case len(t.Skipped) > 0 && len(t.Kept) == 0:
		verb, records = dispositionSkipped, t.Skipped
	case len(t.Kept) > 0 && len(t.Skipped) == 0:
		verb, records = dispositionKept, t.Kept
	default:
		return "", noDisposition, false
	}
	text = taxonomySummaryText(verb, core.TallyOf(records))
	if txt.Cells(text) > warningInlineMaxCells {
		return "", noDisposition, false
	}
	return text, verb, true
}

// inlineTaskFact mirrors inlineTaskWarning at info severity (P8): a Done
// task with no summary, no warnings, and exactly one Fact inlines that
// fact's "name  value" text on its own row instead of nesting it — the same
// "one short annotation inlines" rule Warn's severity already follows,
// applied to Fact's severity too (one placement rule, two severities).
func inlineTaskFact(t core.TaskSnapshot) (core.Fact, bool) {
	if t.State != core.Done || t.Summary != "" || len(t.Warnings) != 0 || len(t.Facts) != 1 {
		return core.Fact{}, false
	}
	f := t.Facts[0]
	if txt.Cells(factText(f)) > warningInlineMaxCells {
		return core.Fact{}, false
	}
	return f, true
}

// inlineFactText renders an inline fact dim, with no attention glyph — a
// Fact is information, never something demanding the reader's attention the
// way an inline warning's "!" does. The leading bangColumnFiller keeps its
// text aligned with a sibling's inline warning/taxonomy text regardless of
// whether such a sibling exists on this render.
func inlineFactText(f core.Fact, color bool) string {
	return bangColumnFiller + txt.Dim(factText(f), color)
}

// writeNestedTaskFacts is inlineTaskFact's nested-line sibling: every fact
// that didn't qualify for inlining renders as its own dim "name  value" line
// under the task's row, the same indentation writeNestedTaskWarnings uses.
func writeNestedTaskFacts(b *strings.Builder, facts []core.Fact, indent string, color bool) {
	for _, f := range facts {
		fmt.Fprintf(b, "%s%s\n", indent, txt.Dim(factText(f), color))
	}
}

// writeNestedTaskWarnings emits warnings that didn't qualify for
// inlineTaskWarning as their own "!" lines under the task's row (P2:
// "multiple/long → nested dim ! lines"), indented by indent (two spaces for
// a standalone task, problemTreeIndent for a collection child). The glyph
// keeps the same yellow attention color writeTaxonomy's "!" rows use — the
// one place a warned task still reads as "not silently clean" in a colored
// terminal, now that its own row glyph is an ordinary green ✓.
func writeNestedTaskWarnings(b *strings.Builder, warnings []core.Problem, indent string, color bool, profile txt.GlyphProfile) {
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
	for _, w := range warnings {
		fmt.Fprintf(b, "%s%s %s\n", indent, glyph, w.Summary)
	}
}

// writeTaxonomy emits the derived "- skipped N (...)" / "! kept N (...)"
// line for a task's accumulated disposition records. Count and reason
// partition are computed here, mechanically, from the records themselves —
// there is nothing for a caller to hand-assemble (and thereby miscount).
// A single reason collapses to its bare name (the count already said N);
// multiple reasons each carry their own count so the parts sum to N.
// indent prefixes the taxonomy row (and, verbose, its detail rows) so a
// collection child nests under its own glyph column and a standalone task
// under its row (taskAnnotationIndent).
// skipSummary is true when the caller already rendered this verb's summary
// text inline on the task's own row (inlineTaskTaxonomy) — the causes
// evidence line and Verbose name list below are unaffected by where the
// headline text landed, so only the summary line itself is suppressed.
func writeTaxonomy(b *strings.Builder, indent string, verb disposition, tally core.Tally, skipSummary, verbose, color bool, profile txt.GlyphProfile) {
	if tally.Total() == 0 {
		return
	}
	if !skipSummary {
		writeTaxonomyHeadline(b, indent, verb, tally, color, profile)
	}
	writeTaxonomyCauses(b, indent, tally.Causes(), verbose, color, profile)
	if !verbose {
		return
	}
	for _, part := range tally.Reasons() {
		fmt.Fprintf(b, "%s%s%s: %s\n", indent, problemDetailIndent, part.Reason, txt.TruncateNames(part.Names, 0, profile))
	}
}

// writeTaxonomyHeadline writes tally's one count line ("- skipped 3
// (...)"), or nothing when it is empty.
func writeTaxonomyHeadline(b *strings.Builder, indent string, verb disposition, tally core.Tally, color bool, profile txt.GlyphProfile) {
	if tally.Total() == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", indent, verb.glyph(color, profile), taxonomySummaryText(verb, tally))
}

// writeDispositions writes d's skipped then kept tallies at indent.
// inlinedVerb names the tally the caller already rendered on its own row
// (inlineTaskTaxonomy), whose headline line is then not repeated; "" when
// noDisposition when none was inlined.
func writeDispositions(b *strings.Builder, indent string, d core.Dispositions, inlinedVerb disposition, verbose, color bool, profile txt.GlyphProfile) {
	writeTaxonomy(b, indent, dispositionSkipped, d.Skipped, inlinedVerb == dispositionSkipped, verbose, color, profile)
	writeTaxonomy(b, indent, dispositionKept, d.Kept, inlinedVerb == dispositionKept, verbose, color, profile)
}

// taskDispositions is t's own two tallies.
func taskDispositions(t core.TaskSnapshot) core.Dispositions {
	var d core.Dispositions
	d.AddTask(&t)
	return d
}

// inlineTaxonomyText is a tally inlined on its task's row, with the same
// glyph its nested line would carry (disposition.glyph).
func inlineTaxonomyText(text string, verb disposition, color bool, profile txt.GlyphProfile) string {
	return txt.Dim(verb.glyph(color, profile)+" "+text, color)
}

// taxonomySummaryText derives the "<verb> N (<reason breakdown>)" text shared
// by a nested taxonomy line and an inlined one (inlineTaskTaxonomy) — one
// place computes the count/reason partition so both placements render
// byte-identical text (fixture-repo-retire-dryrun.md's "kept 13 (8 protected,
// 5 unpushed)": single space before the parenthesis, not the two-space form
// the pre-fixture rendering used).
func taxonomySummaryText(verb disposition, tally core.Tally) string {
	reasons := tally.Reasons()
	parts := make([]string, len(reasons))
	for i, part := range reasons {
		if len(reasons) == 1 {
			parts[i] = part.Reason
			continue
		}
		parts[i] = fmt.Sprintf("%d %s", len(part.Names), part.Reason)
	}
	return fmt.Sprintf("%s %d (%s)", verb, tally.Total(), strings.Join(parts, ", "))
}

// writeTaxonomyCauses renders a tally's accumulated Causes as evidence
// under the count row: one bounded └─ line normally (first cause + "(+N
// more)"), the full list under Verbose (one line per cause).
func writeTaxonomyCauses(b *strings.Builder, indent string, causes []string, verbose, color bool, profile txt.GlyphProfile) {
	if len(causes) == 0 {
		return
	}
	evidence := txt.Dim(txt.GlyphEvidence.Render(profile), color)
	if !verbose {
		line := causes[0]
		if more := len(causes) - 1; more > 0 {
			line = fmt.Sprintf("%s (+%d more)", line, more)
		}
		fmt.Fprintf(b, "%s%s%s %s\n", indent, problemTreeIndent, evidence, line)
		return
	}
	fmt.Fprintf(b, "%s%s%s %s\n", indent, problemTreeIndent, evidence, causes[0])
	for _, c := range causes[1:] {
		fmt.Fprintf(b, "%s%s%s\n", indent, problemDetailIndent, c)
	}
}

// writeRunAnnotations renders evo.Warn/evo.Fact's run-scoped annotations
// (P8 symmetry with a task's own Warn/Fact) — fire-and-forget durable dim
// lines, warnings first: "! <text>" then "<name>  <value>", in call order
// within each severity.
func writeRunAnnotations(b *strings.Builder, warnings []core.Problem, facts []core.Fact, color bool, profile txt.GlyphProfile) {
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
	for _, w := range warnings {
		fmt.Fprintf(b, "%s %s\n", glyph, w.Summary)
	}
	for _, f := range facts {
		fmt.Fprintf(b, "%s\n", txt.Dim(factText(f), color))
	}
}

// factText renders a Fact's "<name>  <value>" text — bare Value alone when
// Name is empty (evo.Fact("", "1 stale")'s value-only spelling), so an
// unnamed fact never carries a spurious leading "  " separator with nothing
// on its left.
func factText(f core.Fact) string {
	if f.Name == "" {
		return f.Value
	}
	return f.Name + "  " + f.Value
}
