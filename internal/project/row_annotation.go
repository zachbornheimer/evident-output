package project

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// WriteTally writes tally's headline and, under Verbose, every item name
// under its reason (items carrying Facts get a row each).
func WriteTally(b *strings.Builder, indent string, tally Tally, s Style) {
	if tally.Empty() {
		return
	}
	WriteTallyHeadline(b, indent, tally, s)
	if !s.Verbose {
		return
	}
	for _, part := range tally.Reasons() {
		if part.HasFacts() {
			writeItemFacts(b, indent+problemDetailIndent, part, s)
			continue
		}
		fmt.Fprintf(b, "%s%s%s: %s\n", indent, problemDetailIndent, part.Reason, strings.Join(part.Names, ", "))
	}
}

// WriteTallyHeadline writes tally's one count line ("○ excluded 3
// (protected)"), or nothing when it is empty.
func WriteTallyHeadline(b *strings.Builder, indent string, tally Tally, s Style) {
	if tally.Empty() {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", indent, s.Dim(GlyphPending.Render(s.Profile)), tallySummaryText(tally))
}

// tallySummaryText derives the "excluded N (<reason breakdown>)" headline:
// one place computes the count and reason partition. A single reason
// collapses to its bare name (the count already said N); several each carry
// their own count so the parts sum to N.
func tallySummaryText(tally Tally) string {
	reasons := tally.Reasons()
	parts := make([]string, len(reasons))
	for i, part := range reasons {
		if len(reasons) == 1 {
			parts[i] = part.Reason
			continue
		}
		parts[i] = fmt.Sprintf("%d %s", len(part.Names), part.Reason)
	}
	return fmt.Sprintf("excluded %d (%s)", tally.Total(), strings.Join(parts, ", "))
}

// WarningText is one run warning's text: its Summary, after the subject it
// is On when it has one ("job  x"), the way a Problem's subject row reads
// (E-109).
func WarningText(w record.Problem) string {
	if w.Subject == "" {
		return w.Summary
	}
	return w.Subject + "  " + w.Summary
}

// inlineTaskFact is the one Fact that belongs directly on t's own row: a Done
// task with no summary and exactly one short Fact inlines that fact's
// "name  value" text instead of nesting it.
func inlineTaskFact(t record.TaskSnapshot) (record.Fact, bool) {
	if !t.State.Succeeded() || t.Summary != "" || len(t.Facts) != 1 {
		return record.Fact{}, false
	}
	f := t.Facts[0]
	if Cells(factText(f)) > warningInlineMaxCells {
		return record.Fact{}, false
	}
	return f, true
}

// inlineFactText renders an inline fact dim, with no attention glyph — a
// Fact is information, never something demanding the reader's attention. It
// is detail text like a Summary, so it starts in the Summary column.
func inlineFactText(f record.Fact, s Style) string {
	return s.Dim(factText(f))
}

// writeNestedTaskFacts is inlineTaskFact's nested-line sibling: every fact
// that didn't qualify for inlining renders as its own dim "name  value" line
// under the task's row.
func writeNestedTaskFacts(b *strings.Builder, facts []record.Fact, indent string, s Style) {
	for _, f := range facts {
		fmt.Fprintf(b, "%s%s\n", indent, s.Dim(factText(f)))
	}
}

// writeItemFacts lists a Reason's items that carry Facts one per line
// under it, each item's Facts at one column past the widest such name, so
// a long name never moves them (E-100). Items without Facts follow as one
// comma-separated list: Verbose names every item.
func writeItemFacts(b *strings.Builder, indent string, part ReasonTally, s Style) {
	fmt.Fprintf(b, "%s%s:\n", indent, part.Reason)
	var rows []int
	var rest []string
	for i, name := range part.Names {
		if len(itemFacts(part, i)) > 0 {
			rows = append(rows, i)
			continue
		}
		rest = append(rest, name)
	}
	width := 0
	for _, i := range rows {
		width = max(width, VisibleCells(part.Names[i]))
	}
	for _, i := range rows {
		facts := itemFacts(part, i)
		pairs := make([]string, len(facts))
		for j, f := range facts {
			pairs[j] = factText(f)
		}
		fmt.Fprintf(b, "%s  %s  %s\n", indent, PadRight(part.Names[i], width), s.Dim(strings.Join(pairs, "  ")))
	}
	if len(rest) > 0 {
		fmt.Fprintf(b, "%s  %s\n", indent, strings.Join(rest, ", "))
	}
}

// itemFacts is the Facts part's i-th item carries.
func itemFacts(part ReasonTally, i int) []record.Fact {
	if i < len(part.Facts) {
		return part.Facts[i]
	}
	return nil
}

// WriteRunAnnotations renders run-scoped warning Problems and evo.Fact
// annotations (P8 symmetry with a task's own Problem/Fact) — fire-and-forget durable dim
// lines, warnings first: "! <text>" then "<name>  <value>", in call order
// within each severity.
func WriteRunAnnotations(b *strings.Builder, warnings []record.Problem, facts []record.Fact, s Style) {
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
func factText(f record.Fact) string {
	if f.Name == "" {
		return f.Value
	}
	return f.Name + "  " + f.Value
}
