package evo

import (
	"fmt"
	"strconv"
	"strings"
)

// compactLayoutMaxWidth switches changes/plans to compact rows.
const compactLayoutMaxWidth = 40

// SGR styles for final-report projection (library-owned sequences only).
const (
	sgrReset  = "\x1b[0m"
	sgrBold   = "\x1b[1m"
	sgrDim    = "\x1b[2m"
	sgrRed    = "\x1b[31m"
	sgrGreen  = "\x1b[32m"
	sgrYellow = "\x1b[33m"
	sgrCyan   = "\x1b[36m"
	sgrBlue   = "\x1b[34m"
)

// PlainOptions configures pure plain projection (§25.4).
type PlainOptions struct {
	Width          int
	NoColor        bool
	NonInteractive bool
}

// RenderPlain projects a snapshot to plain text without terminal ownership.
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	cfg := config{
		width:          opts.Width,
		noColor:        opts.NoColor,
		nonInteractive: opts.NonInteractive,
		plain:          true,
	}
	return []byte(renderPlain(s, cfg)), nil
}

func renderPlain(s Snapshot, cfg config) string {
	var b strings.Builder
	width := cfg.width
	if width <= 0 {
		width = defaultWidth
	}
	color := !cfg.noColor

	for _, line := range s.Lines {
		writeDebugOrLine(&b, line, color)
	}

	for _, it := range s.Items {
		writeItem(&b, it, color)
	}

	for _, t := range s.Tasks {
		writeTask(&b, t, color)
	}

	for _, col := range s.Collections {
		writeCollection(&b, col, color)
	}

	for _, ch := range s.Changes {
		writeEffects(&b, "changed", ch.Subject, ch.Records, width, color)
	}
	for _, p := range s.Plans {
		writeEffects(&b, "planned", p.Subject, p.Records, width, color)
	}

	if s.Conclusion != nil {
		writeConclusion(&b, *s.Conclusion, color)
	}

	return b.String()
}

func writeItem(b *strings.Builder, it ItemSnapshot, color bool) {
	writeItemCore(b, it, color)
	if it.Because != "" {
		writeItemBecause(b, it.Because, color)
	}
	for _, a := range it.Actions {
		writeAction(b, a, color)
	}
}

// writeItemCore emits glyph, name, and problems (terminal outcome body).
func writeItemCore(b *strings.Builder, it ItemSnapshot, color bool) {
	glyph := styleGlyph(itemGlyph(it.State), stateColor(it.State), color)
	fmt.Fprintf(b, "%s  %s\n", glyph, it.Name)
	for _, p := range it.Problems {
		writeProblem(b, p, color)
	}
}

func writeItemBecause(b *strings.Builder, because string, color bool) {
	fmt.Fprintf(b, "  %s\n", dim(because, color))
}

func writeProblem(b *strings.Builder, p Problem, color bool) {
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
		}
		fmt.Fprintf(b, "   %s %s  %s\n", dim("├─", color), p.Subject, extra)
		return
	}
	line := p.Summary
	if p.Detail != "" {
		line = p.Summary + "     " + dim(p.Detail, color)
	}
	fmt.Fprintf(b, "   %s %s\n", dim("└─", color), line)
}

func writeAction(b *strings.Builder, a Action, color bool) {
	if a.Command != nil {
		cmd := a.Command.Executable + " " + strings.Join(a.Command.Args, " ")
		fmt.Fprintf(b, "  %s\n", style(cmd, sgrCyan, color))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "  %s\n", a.Label)
	}
}

func writeTask(b *strings.Builder, t TaskSnapshot, color bool) {
	glyph := styleGlyph(taskGlyph(t.State), stateColor(t.State), color)
	label := t.Name
	switch {
	case t.Summary != "":
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, dim(t.Summary, color))
	case t.Phase != "" && t.State == Running:
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, dim(t.Phase, color))
	default:
		fmt.Fprintf(b, "%s  %s\n", glyph, label)
	}
	// Problems (including Detail from Capture tails) always follow the row.
	// Early-return on Summary used to drop Fail Detail — a silent dialect hole.
	for _, p := range t.Problems {
		writeProblem(b, p, color)
	}
}

func writeCollection(b *strings.Builder, col TasksSnapshot, color bool) {
	glyph := styleGlyph(taskGlyph(col.State), stateColor(col.State), color)
	if col.Summary != "" && col.State == Done {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, col.Name, dim(col.Summary, color))
		return
	}
	fmt.Fprintf(b, "%s  %s\n", glyph, col.Name)
	for _, t := range col.Tasks {
		if t.State == Failed {
			fg := styleGlyph("✗", sgrRed, color)
			fmt.Fprintf(b, "   %s  %s", fg, t.Name)
			if len(t.Problems) > 0 {
				fmt.Fprintf(b, "  %s", t.Problems[0].Summary)
			} else if t.Summary != "" {
				fmt.Fprintf(b, "  %s", t.Summary)
			}
			b.WriteByte('\n')
		}
	}
}

func writeEffects(b *strings.Builder, kind, subject string, records []EffectRecord, width int, color bool) {
	tag := style(fmt.Sprintf("[%s]", kind), effectColor(kind), color)
	fmt.Fprintf(b, "%s  %s\n", tag, subject)
	// TXT-016: leaders omitted when unnecessary (single short column / narrow).
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
	// Bound leader fill so wide verbs do not create unbounded gaps (TXT-016).
	const maxLeader = 12
	for _, r := range records {
		verb := padRight(r.Verb, maxVerb)
		if r.HasQty {
			qty := padLeft(strconv.FormatInt(r.Quantity, 10), maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qty, r.Object)
			continue
		}
		gap := maxVerb - len(r.Verb)
		if gap > maxLeader {
			gap = maxLeader
		}
		if gap > 2 {
			leader := strings.Repeat("·", gap)
			fmt.Fprintf(b, "  %s%s %s\n", r.Verb, dim(leader, color), r.Object)
		} else {
			qtyPad := padLeft("", maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qtyPad, r.Object)
		}
	}
}

func writeConclusion(b *strings.Builder, c Conclusion, color bool) {
	subject := c.Subject
	if subject == "" {
		subject = string(c.State)
	}
	tag := style(fmt.Sprintf("[%s]", c.State), conclusionColor(c.State), color)
	fmt.Fprintf(b, "\n%s  %s\n", tag, style(subject, sgrBold, color))
	if c.Explanation != "" {
		fmt.Fprintf(b, "  %s\n", c.Explanation)
	}
	for _, a := range c.Actions {
		writeAction(b, a, color)
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

func stateColor(s EntityState) string {
	switch s {
	case OK, Done:
		return sgrGreen
	case Failed:
		return sgrRed
	case Blocked:
		return sgrRed
	case Warning:
		return sgrYellow
	case Running:
		return sgrCyan
	case Pending, Skipped, Cancelled, Unknown, Incomplete:
		return sgrDim
	default:
		return ""
	}
}

func conclusionColor(s ConclusionState) string {
	switch s {
	case StateReady, StateUnchanged, StateChanged:
		return sgrGreen
	case StateFailed:
		return sgrRed
	case StateBlocked:
		return sgrRed
	case StatePartial:
		return sgrYellow
	case StateCancelled:
		return sgrDim
	default:
		return sgrCyan
	}
}

func effectColor(kind string) string {
	switch kind {
	case "changed":
		return sgrGreen
	case "planned":
		return sgrBlue
	default:
		return sgrCyan
	}
}

func style(s, code string, color bool) string {
	if !color || code == "" || s == "" {
		return s
	}
	return code + s + sgrReset
}

func styleGlyph(glyph, code string, color bool) string {
	return style(glyph, code, color)
}

func dim(s string, color bool) string {
	return style(s, sgrDim, color)
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
