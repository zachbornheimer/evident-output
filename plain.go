package evo

import (
	"fmt"
	"strconv"
	"strings"
)

// compactLayoutMaxWidth switches changes/plans to compact rows.
const compactLayoutMaxWidth = 40

func renderPlain(s Snapshot, cfg config) string {
	var b strings.Builder
	width := cfg.width
	if width <= 0 {
		width = defaultWidth
	}

	for _, line := range s.Lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	for _, it := range s.Items {
		writeItem(&b, it)
	}

	for _, t := range s.Tasks {
		writeTask(&b, t)
	}

	for _, col := range s.Collections {
		writeCollection(&b, col)
	}

	for _, ch := range s.Changes {
		writeEffects(&b, "changed", ch.Subject, ch.Records, width)
	}
	for _, p := range s.Plans {
		writeEffects(&b, "planned", p.Subject, p.Records, width)
	}

	if s.Conclusion != nil {
		writeConclusion(&b, *s.Conclusion)
	}

	return b.String()
}

func writeItem(b *strings.Builder, it ItemSnapshot) {
	glyph := itemGlyph(it.State)
	fmt.Fprintf(b, "%s  %s\n", glyph, it.Name)
	for _, p := range it.Problems {
		writeProblem(b, p)
	}
	if it.Because != "" {
		fmt.Fprintf(b, "  %s\n", it.Because)
	}
	for _, a := range it.Actions {
		writeAction(b, a)
	}
}

func writeProblem(b *strings.Builder, p Problem) {
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			unit := p.Unit
			if unit == "" {
				unit = "item"
			}
			// plural-ish: keep unit as provided
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
			if p.Unit != "" {
				extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count) // unit shown if needed
				_ = unit
			}
		}
		fmt.Fprintf(b, "   ├─ %s  %s\n", p.Subject, extra)
		return
	}
	line := p.Summary
	if p.Detail != "" {
		line = p.Summary + "     " + p.Detail
	}
	fmt.Fprintf(b, "   └─ %s\n", line)
}

func writeAction(b *strings.Builder, a Action) {
	if a.Command != nil {
		fmt.Fprintf(b, "  %s %s\n", a.Command.Executable, strings.Join(a.Command.Args, " "))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "  %s\n", a.Label)
	}
}

func writeTask(b *strings.Builder, t TaskSnapshot) {
	glyph := taskGlyph(t.State)
	label := t.Name
	if t.Summary != "" {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, t.Summary)
		return
	}
	if t.Phase != "" && t.State == Running {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, t.Phase)
		return
	}
	fmt.Fprintf(b, "%s  %s\n", glyph, label)
	for _, p := range t.Problems {
		writeProblem(b, p)
	}
}

func writeCollection(b *strings.Builder, col TasksSnapshot) {
	glyph := taskGlyph(col.State)
	if col.Summary != "" && col.State == Done {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, col.Name, col.Summary)
		return
	}
	// On failure, do not render success summary (handled by displaySummary).
	fmt.Fprintf(b, "%s  %s\n", glyph, col.Name)
	for _, t := range col.Tasks {
		if t.State == Failed {
			fmt.Fprintf(b, "   ✗  %s", t.Name)
			if len(t.Problems) > 0 {
				fmt.Fprintf(b, "  %s", t.Problems[0].Summary)
			} else if t.Summary != "" {
				fmt.Fprintf(b, "  %s", t.Summary)
			}
			b.WriteByte('\n')
		}
	}
}

func writeEffects(b *strings.Builder, kind, subject string, records []EffectRecord, width int) {
	fmt.Fprintf(b, "[%s]  %s\n", kind, subject)
	if width > 0 && width < compactLayoutMaxWidth {
		for _, r := range records {
			if r.HasQty {
				fmt.Fprintf(b, "  %s %d %s\n", r.Verb, r.Quantity, r.Object)
			} else {
				fmt.Fprintf(b, "  %s %s\n", r.Verb, r.Object)
			}
		}
		return
	}
	maxVerb := 0
	maxQty := 0
	for _, r := range records {
		if len(r.Verb) > maxVerb {
			maxVerb = len(r.Verb)
		}
		if r.HasQty {
			n := len(strconv.FormatInt(r.Quantity, 10))
			if n > maxQty {
				maxQty = n
			}
		}
	}
	for _, r := range records {
		verb := padRight(r.Verb, maxVerb)
		if r.HasQty {
			qty := padLeft(strconv.FormatInt(r.Quantity, 10), maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qty, r.Object)
		} else {
			qtyPad := padLeft("", maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qtyPad, r.Object)
		}
	}
}

func writeConclusion(b *strings.Builder, c Conclusion) {
	subject := c.Subject
	if subject == "" {
		subject = string(c.State)
	}
	fmt.Fprintf(b, "\n[%s]  %s\n", c.State, subject)
	if c.Explanation != "" {
		fmt.Fprintf(b, "  %s\n", c.Explanation)
	}
	for _, a := range c.Actions {
		writeAction(b, a)
	}
}

func itemGlyph(s EntityState) string {
	switch s {
	case OK:
		return "✓"
	case Blocked, Failed:
		return "✗"
	case Warning:
		return "!"
	case Skipped:
		return "○"
	case Unknown, Incomplete:
		return "?"
	case Running:
		return "⠋"
	default:
		return "·"
	}
}

func taskGlyph(s EntityState) string {
	switch s {
	case Done:
		return "✓"
	case Failed:
		return "✗"
	case Warning:
		return "!"
	case Running:
		return "⠋"
	case Pending:
		return "○"
	case Cancelled, Skipped:
		return "○"
	default:
		return "·"
	}
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}
