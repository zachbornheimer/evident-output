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
	msg := warningText(t.Warnings[0])
	if txt.Cells(msg) > warningInlineMaxCells {
		return "", false
	}
	return msg, true
}

// warningText is one warning's text on any row: its Summary, after the
// subject it is On when it has one ("job  x"), the way a Problem's
// subject row reads (E-109).
func warningText(w core.Problem) string {
	if w.Subject == "" {
		return w.Summary
	}
	return w.Subject + "  " + w.Summary
}

// inlineWarningText renders an inline warning with the same "! " bang the
// nested writeNestedTaskWarnings line uses (E2.5 finding 3): the normative
// repo-retire dry-run fixture inlines a warning as "! 2 remotes unreachable" — an
// inline and a nested warning must signal identically, never a dim-only
// inline row that drops the one glyph the fixture treats as load-bearing.
func inlineWarningText(msg string, s Style) string {
	return s.dim(s.warningGlyph() + " " + msg)
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
func inlineFactText(f core.Fact, s Style) string {
	return bangColumnFiller + s.dim(factText(f))
}

// writeNestedTaskFacts is inlineTaskFact's nested-line sibling: every fact
// that didn't qualify for inlining renders as its own dim "name  value" line
// under the task's row, the same indentation writeNestedTaskWarnings uses.
func writeNestedTaskFacts(b *strings.Builder, facts []core.Fact, indent string, s Style) {
	for _, f := range facts {
		fmt.Fprintf(b, "%s%s\n", indent, s.dim(factText(f)))
	}
}

// writeNestedTaskWarnings emits warnings that didn't qualify for
// inlineTaskWarning as their own "!" lines under the task's row (P2:
// "multiple/long → nested dim ! lines"), indented by indent (two spaces for
// a standalone task, problemTreeIndent for a collection child). The glyph
// keeps the same yellow attention color writeTaxonomy's "!" rows use — the
// one place a warned task still reads as "not silently clean" in a colored
// terminal, now that its own row glyph is an ordinary green ✓.
func writeNestedTaskWarnings(b *strings.Builder, warnings []core.Problem, indent string, s Style) {
	glyph := s.warningGlyph()
	for _, w := range warnings {
		fmt.Fprintf(b, "%s%s %s\n", indent, glyph, warningText(w))
	}
}

// writeTaxonomy emits the derived "- skipped N (...)" line for a task's
// accumulated disposition records. Count and reason
// partition are computed here, mechanically, from the records themselves —
// there is nothing for a caller to hand-assemble (and thereby miscount).
// A single reason collapses to its bare name (the count already said N);
// multiple reasons each carry their own count so the parts sum to N.
// indent prefixes the taxonomy row (and, verbose, its detail rows) so a
// collection child nests under its own glyph column and a standalone task
// under its row (taskAnnotationIndent).
func writeTaxonomy(b *strings.Builder, indent string, tally core.Tally, s Style) {
	if tally.Total() == 0 {
		return
	}
	writeTaxonomyHeadline(b, indent, tally, s)
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
func writeItemFacts(b *strings.Builder, indent string, part core.ReasonTally, s Style) {
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
		fmt.Fprintf(b, "%s  %s  %s\n", indent, txt.PadRight(part.Names[i], width), s.dim(strings.Join(pairs, "  ")))
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

// writeTaxonomyHeadline writes tally's one count line ("- skipped 3
// (...)"), or nothing when it is empty.
func writeTaxonomyHeadline(b *strings.Builder, indent string, tally core.Tally, s Style) {
	if tally.Total() == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", indent, skippedTaxonomyGlyph(s), taxonomySummaryText(tally))
}

// taskTally is t's own Skipped records, folded into a Tally.
func taskTally(t core.TaskSnapshot) core.Tally {
	var tally core.Tally
	tally.AddTask(&t)
	return tally
}

// taxonomySummaryText derives the "skipped N (<reason breakdown>)" text
// — one place computes the count/reason partition so both placements
// render byte-identical text ("skipped 419 (283 checked out, 135 unpushed,
// 1 protected)", contract §18: single space before the parenthesis, not the
// two-space form the pre-fixture rendering used). Skipped is the only
// disposition (contract Vocabulary), so the verb is always "skipped".
func taxonomySummaryText(tally core.Tally) string {
	reasons := tally.Reasons()
	parts := make([]string, len(reasons))
	for i, part := range reasons {
		if len(reasons) == 1 {
			parts[i] = part.Reason
			continue
		}
		parts[i] = fmt.Sprintf("%d %s", len(part.Names), part.Reason)
	}
	return fmt.Sprintf("skipped %d (%s)", tally.Total(), strings.Join(parts, ", "))
}

// writeTaxonomyCauses renders a tally's accumulated Causes as evidence
// under the count row: one bounded └─ line normally (first cause + "(+N
// more)"), the full list under Verbose (one line per cause).
func writeTaxonomyCauses(b *strings.Builder, indent string, causes []string, s Style) {
	if len(causes) == 0 {
		return
	}
	evidence := s.evidenceGlyph()
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
func writeRunAnnotations(b *strings.Builder, warnings []core.Problem, facts []core.Fact, s Style) {
	glyph := s.warningGlyph()
	for _, w := range warnings {
		fmt.Fprintf(b, "%s %s\n", glyph, warningText(w))
	}
	for _, f := range facts {
		fmt.Fprintf(b, "%s\n", s.dim(factText(f)))
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
