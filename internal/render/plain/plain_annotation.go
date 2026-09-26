package plain

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/render"
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
	msg := WarningText(t.Warnings[0])
	if txt.Cells(msg) > warningInlineMaxCells {
		return "", false
	}
	return msg, true
}

// WarningText is one warning's text on any row: its Summary, after the
// subject it is On when it has one ("job  x"), the way a Problem's
// subject row reads (E-109).
func WarningText(w core.Problem) string {
	if w.Subject == "" {
		return w.Summary
	}
	return w.Subject + "  " + w.Summary
}

// inlineWarningText renders an inline warning with the same "! " bang the
// nested WriteNestedTaskWarnings line uses (E2.5 finding 3): the normative
// repo-retire dry-run fixture inlines a warning as "! kept 13 (...)" — an
// inline and a nested warning must signal identically, never a dim-only
// inline row that drops the one glyph the fixture treats as load-bearing.
func inlineWarningText(msg string, s render.Style) string {
	return s.Dim(s.WarningGlyph() + " " + msg)
}

// inlineTaskTaxonomy mirrors inlineTaskWarning/inlineTaskFact for a task's
// accumulated Kept/Skipped render.Disposition records (fixture-repo-retire-dryrun.md:
// "✓ branches          ! kept 13 (8 protected, 5 unpushed)" — a taxonomy
// tally IS a warning in the unified annotation model, so it competes for the
// same one-inline-annotation-per-row slot and is disqualified by the same
// conditions: a Summary, an explicit warning-severity Problem, or an
// explicit Fact already claims the row. Both dispositions accumulated at
// once still nest below (rare, and
// two summaries cannot share one inline slot). The returned verb tells the
// caller which of Skipped/Kept was inlined, so its causes/Verbose name list
// (writeTaxonomy's other output) still renders below the row — inlining
// only replaces the Headline count line, never the evidence under it.
func inlineTaskTaxonomy(t core.TaskSnapshot) (text string, verb render.Disposition, ok bool) {
	if t.State != core.Done || t.Summary != "" || len(t.Warnings) != 0 || len(t.Facts) != 0 {
		return "", render.NoDisposition, false
	}
	var records []core.TaxonomyRecord
	switch {
	case len(t.Skipped) > 0 && len(t.Kept) == 0:
		verb, records = render.DispositionSkipped, t.Skipped
	case len(t.Kept) > 0 && len(t.Skipped) == 0:
		verb, records = render.DispositionKept, t.Kept
	default:
		return "", render.NoDisposition, false
	}
	text = taxonomySummaryText(verb, core.TallyOf(records))
	if txt.Cells(text) > warningInlineMaxCells {
		return "", render.NoDisposition, false
	}
	return text, verb, true
}

// inlineTaskFact mirrors inlineTaskWarning at info severity (P8): a Done
// task with no summary, no warnings, and exactly one Fact inlines that
// fact's "name  value" text on its own row instead of nesting it — the same
// "one short annotation inlines" rule the warning severity already follows,
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
func inlineFactText(f core.Fact, s render.Style) string {
	return bangColumnFiller + s.Dim(factText(f))
}

// writeNestedTaskFacts is inlineTaskFact's nested-line sibling: every fact
// that didn't qualify for inlining renders as its own dim "name  value" line
// under the task's row, the same indentation WriteNestedTaskWarnings uses.
func writeNestedTaskFacts(b *strings.Builder, facts []core.Fact, indent string, s render.Style) {
	for _, f := range facts {
		fmt.Fprintf(b, "%s%s\n", indent, s.Dim(factText(f)))
	}
}

// WriteNestedTaskWarnings emits warnings that didn't qualify for
// inlineTaskWarning as their own "!" lines under the task's row (P2:
// "multiple/long → nested dim ! lines"), indented by indent (two spaces for
// a standalone task, problemTreeIndent for a collection child). The glyph
// keeps the same yellow attention color writeTaxonomy's "!" rows use — the
// one place a warned task still reads as "not silently clean" in a colored
// terminal, now that its own row glyph is an ordinary green ✓.
func WriteNestedTaskWarnings(b *strings.Builder, warnings []core.Problem, indent string, s render.Style) {
	glyph := s.WarningGlyph()
	for _, w := range warnings {
		fmt.Fprintf(b, "%s%s %s\n", indent, glyph, WarningText(w))
	}
}

// writeTaxonomy emits the derived "- skipped N (...)" / "! kept N (...)"
// line for a task's accumulated render.Disposition records. Count and reason
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
// Headline text landed, so only the summary line itself is suppressed.
func writeTaxonomy(b *strings.Builder, indent string, verb render.Disposition, tally core.Tally, skipSummary bool, s render.Style) {
	if tally.Total() == 0 {
		return
	}
	if !skipSummary {
		WriteTaxonomyHeadline(b, indent, verb, tally, s)
	}
	writeTaxonomyCauses(b, indent, tally.Causes(), s)
	if !s.Verbose {
		return
	}
	for _, part := range tally.Reasons() {
		if part.HasFacts() {
			writeItemFacts(b, indent+problemDetailIndent, part, s)
			continue
		}
		fmt.Fprintf(b, "%s%s%s: %s\n", indent, problemDetailIndent, part.Reason, txt.TruncateNames(part.Names, 0, s.Profile))
	}
}

// writeItemFacts lists a Reason's items that carry Facts one per line
// under it, each item's Facts at one column past the widest such name, so
// a long name never moves them (E-100). Only the first
// txt.DefaultVisibleNames of them get a row; every other item folds into
// the same bounded "a, b, c … +N more" list a Reason without Facts shows,
// so one Fact never turns a thousand kept items into a thousand lines.
func writeItemFacts(b *strings.Builder, indent string, part core.ReasonTally, s render.Style) {
	fmt.Fprintf(b, "%s%s:\n", indent, part.Reason)
	var rows []int
	var rest []string
	for i, name := range part.Names {
		if len(itemFacts(part, i)) > 0 && len(rows) < txt.DefaultVisibleNames {
			rows = append(rows, i)
			continue
		}
		rest = append(rest, name)
	}
	width := 0
	for _, i := range rows {
		width = max(width, txt.VisibleCells(part.Names[i]))
	}
	for _, i := range rows {
		facts := itemFacts(part, i)
		pairs := make([]string, len(facts))
		for j, f := range facts {
			pairs[j] = factText(f)
		}
		fmt.Fprintf(b, "%s  %s  %s\n", indent, txt.PadRight(part.Names[i], width), s.Dim(strings.Join(pairs, "  ")))
	}
	if len(rest) > 0 {
		fmt.Fprintf(b, "%s  %s\n", indent, txt.TruncateNames(rest, 0, s.Profile))
	}
}

// itemFacts is the Facts part's i-th item carries.
func itemFacts(part core.ReasonTally, i int) []core.Fact {
	if i < len(part.Facts) {
		return part.Facts[i]
	}
	return nil
}

// WriteTaxonomyHeadline writes tally's one count line ("- skipped 3
// (...)"), or nothing when it is empty.
func WriteTaxonomyHeadline(b *strings.Builder, indent string, verb render.Disposition, tally core.Tally, s render.Style) {
	if tally.Total() == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", indent, verb.Glyph(s), taxonomySummaryText(verb, tally))
}

// WriteDispositions writes d's skipped then kept tallies at indent.
// inlinedVerb names the tally the caller already rendered on its own row
// (inlineTaskTaxonomy), whose Headline line is then not repeated; "" when
// render.NoDisposition when none was inlined.
func WriteDispositions(b *strings.Builder, indent string, d core.Dispositions, inlinedVerb render.Disposition, s render.Style) {
	writeTaxonomy(b, indent, render.DispositionSkipped, d.Skipped, inlinedVerb == render.DispositionSkipped, s)
	writeTaxonomy(b, indent, render.DispositionKept, d.Kept, inlinedVerb == render.DispositionKept, s)
}

// TaskDispositions is t's own two tallies.
func TaskDispositions(t core.TaskSnapshot) core.Dispositions {
	var d core.Dispositions
	d.AddTask(&t)
	return d
}

// inlineTaxonomyText is a tally inlined on its task's row, with the same
// glyph its nested line would carry (render.Disposition.glyph).
func inlineTaxonomyText(text string, verb render.Disposition, s render.Style) string {
	return s.Dim(verb.Glyph(s) + " " + text)
}

// taxonomySummaryText derives the "<verb> N (<reason breakdown>)" text shared
// by a nested taxonomy line and an inlined one (inlineTaskTaxonomy) — one
// place computes the count/reason partition so both placements render
// byte-identical text (fixture-repo-retire-dryrun.md's "kept 13 (8 protected,
// 5 unpushed)": single space before the parenthesis, not the two-space form
// the pre-fixture rendering used).
func taxonomySummaryText(verb render.Disposition, tally core.Tally) string {
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
func writeTaxonomyCauses(b *strings.Builder, indent string, causes []string, s render.Style) {
	if len(causes) == 0 {
		return
	}
	evidence := s.EvidenceGlyph()
	if !s.Verbose {
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

// writeRunAnnotations renders evo.Problem's warning-severity results and
// evo.Fact's run-scoped annotations (P8 symmetry with a task's own
// Problem/Fact) — fire-and-forget durable dim
// lines, warnings first: "! <text>" then "<name>  <value>", in call order
// within each severity.
func writeRunAnnotations(b *strings.Builder, warnings []core.Problem, facts []core.Fact, s render.Style) {
	glyph := s.WarningGlyph()
	for _, w := range warnings {
		fmt.Fprintf(b, "%s %s\n", glyph, WarningText(w))
	}
	for _, f := range facts {
		fmt.Fprintf(b, "%s\n", s.Dim(factText(f)))
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
