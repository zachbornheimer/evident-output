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
	// Verbose additionally emits per-reason name lists under a task's skip/keep
	// taxonomy line. Counts and the reason partition always render; Verbose
	// only adds the bounded (TruncateNames) name detail.
	Verbose bool
	// Glyphs selects the state-glyph vocabulary. Plain projection has no live
	// TTY to detect, so GlyphsAuto (the default) resolves to GlyphsUnicode —
	// callers rendering off a known non-UTF-8 destination pass GlyphsASCII.
	Glyphs GlyphProfile
}

// RenderPlain projects a snapshot to plain text without terminal ownership.
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	glyphs := opts.Glyphs
	if glyphs == GlyphsAuto {
		glyphs = GlyphsUnicode
	}
	cfg := config{
		width:          opts.Width,
		noColor:        opts.NoColor,
		nonInteractive: opts.NonInteractive,
		plain:          true,
		glyphs:         glyphs,
	}
	if opts.Verbose {
		cfg.verbosity = VerbosityVerbose
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
	verbose := cfg.verbosity >= VerbosityVerbose
	profile := cfg.glyphs

	if s.DryRun {
		writeDryRunMarker(&b, color)
	}

	for _, line := range s.Lines {
		writeDebugOrLine(&b, line, color)
	}

	for _, it := range s.Items {
		writeItem(&b, it, color, profile)
	}

	for _, t := range s.Tasks {
		writeTask(&b, t, color, verbose, profile)
	}

	for _, col := range s.Collections {
		writeCollection(&b, col, color, profile)
	}

	for _, ch := range s.Changes {
		writeEffects(&b, "changed", ch.Subject, ch.Records, width, color)
	}
	for _, p := range s.Plans {
		writeEffects(&b, "planned", p.Subject, p.Records, width, color)
	}

	if s.Conclusion != nil && !shouldSuppressStandaloneConclusion(s) {
		writeConclusion(&b, *s.Conclusion, color)
	}

	return b.String()
}

func writeItem(b *strings.Builder, it ItemSnapshot, color bool, profile GlyphProfile) {
	writeItemCore(b, it, color, profile)
	if it.Because != "" {
		writeItemBecause(b, it.Because, color)
	}
	for _, a := range it.Actions {
		writeAction(b, a, color)
	}
}

// maxVisibleProblems is the human default bound (OPEN-003). Structured snapshots
// retain full Problem lists separately (DEC-FAIL-001/003).
const maxVisibleProblems = 5

// writeItemCore emits glyph, name, and problems (terminal outcome body).
func writeItemCore(b *strings.Builder, it ItemSnapshot, color bool, profile GlyphProfile) {
	glyph := styleGlyph(itemGlyph(it.State, profile), stateColor(it.State), color)
	fmt.Fprintf(b, "%s  %s\n", glyph, it.Name)
	problems := it.Problems
	omitted := 0
	if len(problems) > maxVisibleProblems {
		omitted = len(problems) - maxVisibleProblems
		problems = problems[:maxVisibleProblems]
	}
	for _, p := range problems {
		writeProblem(b, p, color)
	}
	if omitted > 0 {
		writeProblem(b, Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		}, color)
	}
}

func writeItemBecause(b *strings.Builder, because string, color bool) {
	fmt.Fprintf(b, "  %s\n", dim(because, color))
}

// Plain problem indent widths (fixed presentation dialect, not operational knobs).
const (
	// problemTreeIndent prefixes ├─ / └─ / │ problem rows.
	problemTreeIndent = "   "
	// problemDetailIndent continues multi-line Detail under a └─ / │ opener.
	problemDetailIndent = "      "
)

func writeProblem(b *strings.Builder, p Problem, color bool) {
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
		}
		fmt.Fprintf(b, "%s%s %s  %s\n", problemTreeIndent, dim("├─", color), p.Subject, extra)
		if p.Detail != "" {
			writeProblemDetailLines(b, p.Detail, dim("│", color), color)
		}
		return
	}
	// Detail present: preserve multi-line body (P3). Summary heads the block only
	// when the caller left it set — writeTask clears Summary when it already
	// appears on the ✗ row so Detail alone is the └─ body (P4).
	if p.Detail != "" {
		writeProblemDetailBlock(b, p.Summary, p.Detail, color)
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, dim("└─", color), p.Summary)
}

// writeProblemDetailBlock renders Detail as an indented multi-line block under └─.
// When summary is non-empty it opens the block; continuations (and all detail
// lines when summary is empty) are indented under it.
func writeProblemDetailBlock(b *strings.Builder, summary, detail string, color bool) {
	lines := splitPresentationLines(detail)
	if summary != "" {
		fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, dim("└─", color), summary)
		for _, line := range lines {
			fmt.Fprintf(b, "%s%s\n", problemDetailIndent, dim(line, color))
		}
		return
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, dim("└─", color), dim(lines[0], color))
	for _, line := range lines[1:] {
		fmt.Fprintf(b, "%s%s\n", problemDetailIndent, dim(line, color))
	}
}

// writeProblemDetailLines continues Detail under a subject (├─) row with │ prefixes.
func writeProblemDetailLines(b *strings.Builder, detail, pipe string, color bool) {
	for _, line := range splitPresentationLines(detail) {
		fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, pipe, dim(line, color))
	}
}

// splitPresentationLines splits on \n and drops a single trailing empty segment
// so a trailing newline does not produce a blank residual row.
func splitPresentationLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
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

func writeTask(b *strings.Builder, t TaskSnapshot, color, verbose bool, profile GlyphProfile) {
	glyph := styleGlyph(taskGlyph(t.State, profile), stateColor(t.State), color)
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
	problems := t.Problems
	omitted := 0
	if len(problems) > maxVisibleProblems {
		omitted = len(problems) - maxVisibleProblems
		problems = problems[:maxVisibleProblems]
	}
	for _, p := range problems {
		// P4: task glyph row already shows t.Summary; do not re-echo it as the
		// └─ header when Detail carries the real evidence (capture tail / diff).
		if p.Detail != "" && p.Summary != "" && p.Summary == t.Summary {
			p.Summary = ""
		}
		writeProblem(b, p, color)
	}
	if omitted > 0 {
		writeProblem(b, Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		}, color)
	}
	writeTaxonomy(b, "skipped", t.Skipped, verbose, color)
	writeTaxonomy(b, "kept", t.Kept, verbose, color)
}

// writeTaxonomy emits the derived "!  skipped N  (...)" / "!  kept N  (...)"
// line for a task's accumulated disposition records. Count and reason
// partition are computed here, mechanically, from the records themselves —
// there is nothing for a caller to hand-assemble (and thereby miscount).
// A single reason collapses to its bare name (the count already said N);
// multiple reasons each carry their own count so the parts sum to N.
func writeTaxonomy(b *strings.Builder, verb string, records []TaxonomyRecord, verbose, color bool) {
	if len(records) == 0 {
		return
	}
	names, order := partitionTaxonomyByReason(records)
	parts := make([]string, len(order))
	for i, reason := range order {
		if len(order) == 1 {
			parts[i] = reason
			continue
		}
		parts[i] = fmt.Sprintf("%d %s", len(names[reason]), reason)
	}
	glyph := styleGlyph("!", sgrYellow, color)
	fmt.Fprintf(b, "%s  %s %d  (%s)\n", glyph, verb, len(records), strings.Join(parts, ", "))
	if !verbose {
		return
	}
	for _, reason := range order {
		fmt.Fprintf(b, "%s%s: %s\n", problemDetailIndent, reason, TruncateNames(names[reason], 0))
	}
}

// partitionTaxonomyByReason groups records by reason, preserving first-seen
// order so the rendered partition matches the order reasons were recorded.
func partitionTaxonomyByReason(records []TaxonomyRecord) (map[string][]string, []string) {
	names := make(map[string][]string)
	var order []string
	for _, r := range records {
		if _, ok := names[r.Reason]; !ok {
			order = append(order, r.Reason)
		}
		names[r.Reason] = append(names[r.Reason], r.Name)
	}
	return names, order
}

// writeCollection renders a Tasks group: the parent glyph/name (with its own
// Summary when set), then every resolved child row with its own summary or
// problem — Done included. Evo-rec.md Problem 1's final ledger keeps ✓ rows
// like "✓  branches   14 deleted" instead of the parent collapsing to one
// line and erasing the children whose evidence lived only in the live
// region while it was running.
func writeCollection(b *strings.Builder, col TasksSnapshot, color bool, profile GlyphProfile) {
	glyph := styleGlyph(taskGlyph(col.State, profile), stateColor(col.State), color)
	if col.Summary != "" {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, col.Name, dim(col.Summary, color))
	} else {
		fmt.Fprintf(b, "%s  %s\n", glyph, col.Name)
	}
	for _, t := range col.Tasks {
		writeCollectionChild(b, t, color, profile)
	}
}

// writeCollectionChild renders one child task row under its parent group:
// glyph, name, and whichever of problem summary / task summary explains it.
func writeCollectionChild(b *strings.Builder, t TaskSnapshot, color bool, profile GlyphProfile) {
	tg := styleGlyph(taskGlyph(t.State, profile), stateColor(t.State), color)
	fmt.Fprintf(b, "   %s  %s", tg, t.Name)
	switch {
	case len(t.Problems) > 0:
		fmt.Fprintf(b, "  %s", t.Problems[0].Summary)
	case t.Summary != "":
		fmt.Fprintf(b, "  %s", t.Summary)
	}
	b.WriteByte('\n')
}

func writeEffects(b *strings.Builder, kind, subject string, records []EffectRecord, width int, color bool) {
	// A [planned]/[changed] header with zero rows beneath it invents a mutation
	// story that never happened; render the honest empty-success line instead
	// (evo-rec.md "nothing-to-do" default).
	if len(records) == 0 {
		fmt.Fprintf(b, "nothing to %s\n", subject)
		return
	}
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

// dryRunMarkerText is the fixed announcement body for a dry-run run's
// opening line (evo-rec.md Problem 1: "a dry run must announce itself").
const dryRunMarkerText = "no changes will be made"

// writeDryRunMarker emits the unmissable dry-run marker line. It renders
// once, first, styled like [planned] — a caller cannot opt out or bury it,
// because every dry-run projection (RenderPlain and Finish's residual) calls
// this before any other row.
func writeDryRunMarker(b *strings.Builder, color bool) {
	tag := style("[dry-run]", effectColor("planned"), color)
	fmt.Fprintf(b, "%s  %s\n", tag, dryRunMarkerText)
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
	case Pending, Skipped, Cancelled, Unknown, Incomplete, NotStarted:
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
