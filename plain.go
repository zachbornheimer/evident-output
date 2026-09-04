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
	Width   int
	NoColor bool
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
		width:   opts.Width,
		noColor: opts.NoColor,
		plain:   true,
		glyphs:  glyphs,
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

	for _, t := range s.Tasks {
		writeTask(&b, t, color, verbose, profile)
	}

	for _, col := range s.Collections {
		writeCollection(&b, col, color, verbose, profile)
	}

	for _, ch := range s.Changes {
		writeEffects(&b, "changed", ch.Subject, ch.Records, ch.IntendedVerb, width, color, profile)
	}
	for _, p := range s.Plans {
		writeEffects(&b, "planned", p.Subject, p.Records, p.IntendedVerb, width, color, profile)
	}

	if s.Conclusion != nil && !shouldSuppressStandaloneConclusion(s) {
		writeConclusion(&b, *s.Conclusion, color, profile)
	}

	return b.String()
}

// maxVisibleProblems is the human default bound (OPEN-003). Structured snapshots
// retain full Problem lists separately (DEC-FAIL-001/003).
const maxVisibleProblems = 5

// Plain problem indent widths (fixed presentation dialect, not operational knobs).
const (
	// problemTreeIndent prefixes ├─ / └─ / │ problem rows.
	problemTreeIndent = "   "
	// problemDetailIndent continues multi-line Detail under a └─ / │ opener.
	problemDetailIndent = "      "
)

// writeProblem renders one problem row. emphasize is true for a Failed/Blocked
// task's evidence (release-gate round 6 finding 5): the ├─/│/evidence-glyph
// connectors stay dim either way — they are decoration — but the evidence
// text itself renders at full intensity so a failure's proof is never the
// lowest-contrast text on screen.
func writeProblem(b *strings.Builder, p Problem, color, emphasize bool, profile GlyphProfile) {
	detail, tail := effectiveDetailAndTail(p)
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
		}
		fmt.Fprintf(b, "%s%s %s  %s\n", problemTreeIndent, dim("├─", color), p.Subject, extra)
		if detail != "" {
			writeProblemDetailLines(b, detail, dim("│", color), color, emphasize)
		}
		if tail != "" {
			writeProblemDetailLines(b, tail, dim("│", color), color, emphasize)
		}
		return
	}
	// Detail present: preserve multi-line body (P3). Summary heads the block only
	// when the caller left it set — writeTask clears Summary when it already
	// appears on the ✗ row so Detail alone is the evidence body (P4).
	if detail != "" {
		writeProblemDetailBlock(b, p.Summary, detail, color, emphasize, profile)
		if tail != "" {
			writeAdditionalEvidenceLines(b, tail, color, emphasize)
		}
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, dim(glyphEvidence.render(profile), color), emphasizedText(p.Summary, color, emphasize))
}

// emphasizedText applies dim unless emphasize opts the text out of demotion —
// the single place that decides "is this text decoration or evidence" for
// the problem-rendering chain.
func emphasizedText(s string, color, emphasize bool) string {
	if emphasize {
		return s
	}
	return dim(s, color)
}

// effectiveDetailAndTail resolves a Problem's Detail and EvidenceTail into
// the pair actually rendered: an explicit Detail always renders (never
// silently discarded by an auto-attached or explicitly requested evidence
// tail), and a distinct EvidenceTail renders as an additional evidence line
// underneath it. When Detail is empty, EvidenceTail alone renders as the
// detail body — DetailTail's original, still-supported shape. An identical
// EvidenceTail (auto-attach filled Detail with the same capture tail a
// caller also passed explicitly via DetailTail) collapses to one line, not a
// duplicate.
func effectiveDetailAndTail(p Problem) (detail, tail string) {
	switch {
	case p.Detail == "":
		return p.EvidenceTail, ""
	case p.EvidenceTail == "" || p.EvidenceTail == p.Detail:
		return p.Detail, ""
	default:
		return p.Detail, p.EvidenceTail
	}
}

// writeAdditionalEvidenceLines renders tail's lines as continuation rows
// under a just-written Detail block, matching writeProblemDetailBlock's own
// continuation indent so the tail reads as more evidence for the same
// problem rather than a new one.
func writeAdditionalEvidenceLines(b *strings.Builder, tail string, color, emphasize bool) {
	for _, line := range splitPresentationLines(tail) {
		fmt.Fprintf(b, "%s%s\n", problemDetailIndent, emphasizedText(line, color, emphasize))
	}
}

// writeProblemDetailBlock renders Detail as an indented multi-line block under
// the evidence connector. When summary is non-empty it opens the block;
// continuations (and all detail lines when summary is empty) are indented
// under it.
func writeProblemDetailBlock(b *strings.Builder, summary, detail string, color, emphasize bool, profile GlyphProfile) {
	lines := splitPresentationLines(detail)
	evidence := dim(glyphEvidence.render(profile), color)
	if summary != "" {
		fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, evidence, emphasizedText(summary, color, emphasize))
		for _, line := range lines {
			fmt.Fprintf(b, "%s%s\n", problemDetailIndent, emphasizedText(line, color, emphasize))
		}
		return
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, evidence, emphasizedText(lines[0], color, emphasize))
	for _, line := range lines[1:] {
		fmt.Fprintf(b, "%s%s\n", problemDetailIndent, emphasizedText(line, color, emphasize))
	}
}

// writeProblemDetailLines continues Detail under a subject (├─) row with │ prefixes.
func writeProblemDetailLines(b *strings.Builder, detail, pipe string, color, emphasize bool) {
	for _, line := range splitPresentationLines(detail) {
		fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, pipe, emphasizedText(line, color, emphasize))
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

// writeAction renders one next-action row prefixed by the profile-aware next-
// action glyph (→ / >). evo-rec.md's tightened vocabulary gives "next action"
// its own row so the meaning does not rest on cyan color alone.
func writeAction(b *strings.Builder, a Action, color bool, profile GlyphProfile) {
	glyph := styleGlyph(glyphNextAction.render(profile), sgrCyan, color)
	if a.Command != nil {
		cmd := a.Command.Executable + " " + strings.Join(a.Command.Args, " ")
		fmt.Fprintf(b, "%s  %s\n", glyph, style(cmd, sgrCyan, color))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "%s  %s\n", glyph, a.Label)
	}
}

func writeTask(b *strings.Builder, t TaskSnapshot, color, verbose bool, profile GlyphProfile) {
	glyph := styleGlyph(taskGlyph(t.State, profile), stateColor(t.State), color)
	label := t.Name
	runningDetail := ""
	if t.State == Running {
		runningDetail = runningTaskDetail(t)
	}
	switch {
	case t.Summary != "" && (t.State == Failed || t.State == Blocked):
		// release-gate round 6 finding 5: a Fail/Block summary is the evidence
		// the reader most needs — it must never render at the lowest contrast
		// on screen. Full intensity here; dim stays for genuinely subordinate
		// outcomes (Done/Skip/Cancel summaries) below.
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, t.Summary)
	case t.Summary != "":
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, dim(t.Summary, color))
	case runningDetail != "":
		// Default intensity, not dim: the in-flight phase/progress is the
		// diagnostic signal while work is stalled — dim is reserved for
		// genuinely subordinate rows (○ pending, - not started, evidence,
		// overflow), per evo-rec.md "Color and style demotions".
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, runningDetail)
	default:
		fmt.Fprintf(b, "%s  %s\n", glyph, label)
	}
	// Problems (including Detail from Capture tails) always follow the row.
	// Early-return on Summary used to drop Fail Detail — a silent dialect hole.
	// emphasize keeps a Fail/Block task's evidence at full intensity (finding 5).
	emphasize := t.State == Failed || t.State == Blocked
	problems := t.Problems
	omitted := 0
	if len(problems) > maxVisibleProblems {
		omitted = len(problems) - maxVisibleProblems
		problems = problems[:maxVisibleProblems]
	}
	for _, p := range problems {
		// beginner-3: the task glyph row already shows t.Summary. A problem
		// row with no Detail beyond that summary says nothing new — drop it
		// entirely instead of re-echoing "└─ <same text>" underneath.
		if p.Detail == "" && p.EvidenceTail == "" && p.Subject == "" && p.Summary != "" && p.Summary == t.Summary {
			continue
		}
		// P4: task glyph row already shows t.Summary; do not re-echo it as the
		// └─ header when Detail carries the real evidence (capture tail / diff).
		if (p.Detail != "" || p.EvidenceTail != "") && p.Summary != "" && p.Summary == t.Summary {
			p.Summary = ""
		}
		writeProblem(b, p, color, emphasize, profile)
	}
	if omitted > 0 {
		writeProblem(b, Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		}, color, emphasize, profile)
	}
	writeTaxonomy(b, "", "skipped", t.Skipped, verbose, color, profile)
	writeTaxonomy(b, "", "kept", t.Kept, verbose, color, profile)
}

// runningTaskDetail composes a Running task's plain-mode detail text: its
// progress count (or Bytes fraction), its phase, or both together
// ("14/40  requests") — the durable-line counterpart of writeLiveTaskLine's
// interactive combination, without the live-only bar/heartbeat decoration.
// Returns "" for a Running task with neither (never happens through the
// public API, since every path that promotes Pending to Running sets one).
func runningTaskDetail(t TaskSnapshot) string {
	count := ""
	switch {
	case t.Progress.Kind == BytesKind && t.Progress.Total > 0:
		count = formatByteProgressFixed(t.Progress.Completed, t.Progress.Total)
	case t.Progress.Kind == Determinate && t.Progress.Total > 0:
		count = fmt.Sprintf("%d/%d", t.Progress.Completed, t.Progress.Total)
	}
	switch {
	case count != "" && t.Phase != "":
		return count + "  " + t.Phase
	case count != "":
		return count
	default:
		return t.Phase
	}
}

// writeTaxonomy emits the derived "!  skipped N  (...)" / "!  kept N  (...)"
// line for a task's accumulated disposition records. Count and reason
// partition are computed here, mechanically, from the records themselves —
// there is nothing for a caller to hand-assemble (and thereby miscount).
// A single reason collapses to its bare name (the count already said N);
// multiple reasons each carry their own count so the parts sum to N.
// indent prefixes the taxonomy row (and, verbose, its detail rows) so a
// collection child's taxonomy nests under the child's own glyph column
// instead of the standalone task's zero-indent column.
func writeTaxonomy(b *strings.Builder, indent, verb string, records []TaxonomyRecord, verbose, color bool, profile GlyphProfile) {
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
	glyph := styleGlyph(glyphWarningState.render(profile), sgrYellow, color)
	fmt.Fprintf(b, "%s%s  %s %d  (%s)\n", indent, glyph, verb, len(records), strings.Join(parts, ", "))
	writeTaxonomyCauses(b, indent, records, verbose, color, profile)
	if !verbose {
		return
	}
	for _, reason := range order {
		fmt.Fprintf(b, "%s%s%s: %s\n", indent, problemDetailIndent, reason, TruncateNames(names[reason], 0, profile))
	}
}

// writeTaxonomyCauses renders every records' accumulated Causes as evidence
// under the count row: one bounded └─ line normally (first cause + "(+N
// more)"), the full list under Verbose (one line per cause).
func writeTaxonomyCauses(b *strings.Builder, indent string, records []TaxonomyRecord, verbose, color bool, profile GlyphProfile) {
	var causes []string
	for _, r := range records {
		causes = append(causes, r.Causes...)
	}
	if len(causes) == 0 {
		return
	}
	evidence := dim(glyphEvidence.render(profile), color)
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
func writeCollection(b *strings.Builder, col TasksSnapshot, color, verbose bool, profile GlyphProfile) {
	glyph := styleGlyph(taskGlyph(col.State, profile), stateColor(col.State), color)
	if col.Summary != "" {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, col.Name, dim(col.Summary, color))
	} else {
		fmt.Fprintf(b, "%s  %s\n", glyph, col.Name)
	}
	for _, t := range col.Tasks {
		writeCollectionChild(b, t, color, verbose, profile)
	}
}

// writeCollectionChild renders one child task row under its parent group:
// glyph, name, and whichever of problem summary / task summary explains it,
// then every problem's Detail/evidence line the same way a standalone
// writeTask already does — a collection child is a task, and dropping its
// evidence (└─ ...) and taxonomy here was the gap that forced the
// repo-retire adoption off the Group/Tasks API.
func writeCollectionChild(b *strings.Builder, t TaskSnapshot, color, verbose bool, profile GlyphProfile) {
	tg := styleGlyph(taskGlyph(t.State, profile), stateColor(t.State), color)
	fmt.Fprintf(b, "   %s  %s", tg, t.Name)
	headerSummary := t.Summary
	if len(t.Problems) > 0 {
		headerSummary = t.Problems[0].Summary
		fmt.Fprintf(b, "  %s", headerSummary)
	} else if headerSummary != "" {
		fmt.Fprintf(b, "  %s", headerSummary)
	}
	b.WriteByte('\n')
	// emphasize keeps a Fail/Block child's evidence at full intensity, the
	// same contrast rule writeTask applies to a standalone task (finding 5).
	emphasize := t.State == Failed || t.State == Blocked
	problems := t.Problems
	omitted := 0
	if len(problems) > maxVisibleProblems {
		omitted = len(problems) - maxVisibleProblems
		problems = problems[:maxVisibleProblems]
	}
	for _, p := range problems {
		// beginner-3: mirror writeTask's de-echo — a problem row with no
		// Detail beyond the already-shown header summary says nothing new.
		if p.Detail == "" && p.EvidenceTail == "" && p.Subject == "" && p.Summary != "" && p.Summary == headerSummary {
			continue
		}
		// The header row already showed this summary; Detail alone is the
		// evidence body (mirrors writeTask's P4 dedup).
		if (p.Detail != "" || p.EvidenceTail != "") && p.Summary != "" && p.Summary == headerSummary {
			p.Summary = ""
		}
		writeProblem(b, p, color, emphasize, profile)
	}
	if omitted > 0 {
		writeProblem(b, Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		}, color, emphasize, profile)
	}
	writeTaxonomy(b, problemTreeIndent, "skipped", t.Skipped, verbose, color, profile)
	writeTaxonomy(b, problemTreeIndent, "kept", t.Kept, verbose, color, profile)
}

// maxVisibleEffectRows bounds how many plan/changes rows the human view
// renders per section (evo-rec.md "bound visible RecordName rows... model
// keeps all records"). The snapshot always retains the full record list;
// only this presentation loop is capped. Mirrors maxVisibleProblems's bound.
const maxVisibleEffectRows = maxVisibleProblems

// ledgerObject renders r.Object pluralized from r.Quantity when the record
// carries a quantity (I4) — mutation verbs take a singular object
// (Delete(2, "stale local branch")) and the ledger derives "branches" at
// render time via Pluralize, instead of every call site hand-composing its
// own singular/plural noun with evo.Pluralize. Pluralize itself stays
// exported for prose outside the ledger (a Printf line, a Problem detail).
func ledgerObject(r EffectRecord) string {
	if !r.HasQty {
		return r.Object
	}
	return Pluralize(r.Quantity, r.Object)
}

// mergeIdenticalEffectRecords combines repeated records that share the same
// (verb, object) pair — one ledger's tense is fixed for every record it
// holds, so verb+object alone identifies a duplicate row — into one row with
// summed quantities: twelve `Delete(1, "merged branch")` calls render
// "deleted 12 merged branches", not twelve identical rows plus an overflow
// ellipsis (release-gate finding 6). Distinct records (different verb or
// object) are left alone and keep their original relative order. The model
// itself is untouched — only this presentation-layer view merges.
func mergeIdenticalEffectRecords(records []EffectRecord) []EffectRecord {
	type key struct{ verb, object string }
	index := make(map[key]int, len(records))
	merged := make([]EffectRecord, 0, len(records))
	for _, r := range records {
		k := key{r.Verb, r.Object}
		if i, ok := index[k]; ok {
			m := &merged[i]
			if !m.HasQty {
				m.HasQty = true
				m.Quantity = 1
			}
			if r.HasQty {
				m.Quantity += r.Quantity
			} else {
				m.Quantity++
			}
			continue
		}
		index[k] = len(merged)
		merged = append(merged, r)
	}
	return merged
}

func writeEffects(b *strings.Builder, kind, subject string, records []EffectRecord, intendedVerb string, width int, color bool, profile GlyphProfile) {
	// A [planned]/[changed] header with zero rows beneath it invents a mutation
	// story that never happened; render the honest empty-success line instead
	// (evo-rec.md "nothing-to-do" default). The verb comes from the section's
	// own recorded intent — never hand-assembled — falling back to a generic
	// phrasing only when no mutation verb was ever recorded for it.
	if len(records) == 0 {
		if intendedVerb != "" {
			fmt.Fprintf(b, "nothing to %s %s\n", intendedVerb, subject)
		} else {
			fmt.Fprintf(b, "nothing to change for %s\n", subject)
		}
		return
	}
	tag := style(fmt.Sprintf("[%s]", kind), effectColor(kind), color)
	fmt.Fprintf(b, "%s  %s\n", tag, subject)

	visible := mergeIdenticalEffectRecords(records)
	omitted := 0
	if len(visible) > maxVisibleEffectRows {
		omitted = len(visible) - maxVisibleEffectRows
		visible = visible[:maxVisibleEffectRows]
	}

	// TXT-016: leaders omitted when unnecessary (single short column / narrow).
	if width > 0 && width < compactLayoutMaxWidth {
		for _, r := range visible {
			if r.HasQty {
				fmt.Fprintf(b, "  %s %d %s\n", r.Verb, r.Quantity, ledgerObject(r))
			} else {
				fmt.Fprintf(b, "  %s %s\n", r.Verb, r.Object)
			}
		}
		writeEffectOverflow(b, omitted, color, profile)
		return
	}
	maxVerb := 0
	maxQty := 0
	for _, r := range visible {
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
	for _, r := range visible {
		verb := padRight(r.Verb, maxVerb)
		if r.HasQty {
			qty := padLeft(strconv.FormatInt(r.Quantity, 10), maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qty, ledgerObject(r))
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
	writeEffectOverflow(b, omitted, color, profile)
}

// writeEffectOverflow renders the bounded-rows omission line. The overflow
// glyph (dim "…"/"...") marks it, not "!" — an omitted-count line is a
// viewport limit, not something demanding attention (evo-rec.md "! is
// attention only... Overflow is never !").
func writeEffectOverflow(b *strings.Builder, omitted int, color bool, profile GlyphProfile) {
	if omitted <= 0 {
		return
	}
	fmt.Fprintf(b, "  %s  +%d more (not shown)\n", dim(glyphOverflow.render(profile), color), omitted)
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

// conclusionPartialModifier is the literal suffix that marks the printed
// band as evo-rec.md's completeness axis rather than a new headline: a run
// that never invented a State of its own (StatePartial is dead precisely
// because Partial is a modifier, not a root verdict) still needs an honest
// band when Conclusion.Partial is true (release-gate round 4 finding 1) — an
// abandoned Each loop or a forgotten terminal verb on an otherwise clean
// finish must not read as silently complete.
const conclusionPartialModifier = " · partial"

// conclusionBandTag renders the trailing outcome band's bracketed tag,
// appending conclusionPartialModifier when the conclusion is incomplete —
// evo-rec.md's spec does not pin a literal spelling for the OK+Partial band
// (only a two-axis rule), so the modifier form here is the documented
// implementation choice (see work order for the corresponding spec-edit note).
func conclusionBandTag(c Conclusion) string {
	if c.Partial {
		return fmt.Sprintf("[%s%s]", c.State, conclusionPartialModifier)
	}
	return fmt.Sprintf("[%s]", c.State)
}

func writeConclusion(b *strings.Builder, c Conclusion, color bool, profile GlyphProfile) {
	subject := c.Subject
	if subject == "" {
		subject = string(c.State)
	}
	tag := style(conclusionBandTag(c), conclusionColor(c.State), color)
	fmt.Fprintf(b, "\n%s  %s\n", tag, style(subject, sgrBold, color))
	if c.Explanation != "" {
		fmt.Fprintf(b, "  %s\n", c.Explanation)
	}
	if c.State == StateCancelled || c.State == StateFailed {
		writeAlreadyMutated(b, c.Changes, color, profile)
	}
	for _, a := range c.Actions {
		writeAction(b, a, color, profile)
	}
}

// writeAlreadyMutated renders the early-termination "! already mutated: ..."
// line. It fires whenever a run concludes Cancelled or Failed with at least
// one committed effect — "!" is attention-only (evo-rec.md "Tightened glyph
// vocabulary"), and an empty ledger earns no attention, so the row is
// suppressed entirely rather than rendered as "none". The summary is derived
// mechanically from the Changes ledger, never assembled by the caller
// (evo-rec.md "Taxonomy and mutation lines are derived, never assembled").
func writeAlreadyMutated(b *strings.Builder, changes []ChangesSnapshot, color bool, profile GlyphProfile) {
	summary, ok := summarizeAlreadyMutated(changes)
	if !ok {
		return
	}
	glyph := styleGlyph(glyphWarningState.render(profile), sgrYellow, color)
	fmt.Fprintf(b, "%s  already mutated: %s\n", glyph, summary)
}

// summarizeAlreadyMutated derives one compact fragment per non-empty Changes
// section (e.g. "8 branches deleted"), joined with "; ". ok is false when no
// section committed any effect, telling the caller to suppress the row.
func summarizeAlreadyMutated(changes []ChangesSnapshot) (string, bool) {
	var parts []string
	for _, ch := range changes {
		if len(ch.Records) == 0 {
			continue
		}
		parts = append(parts, summarizeChangeSection(ch))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "; "), true
}

// summarizeChangeSection sums a section's record quantities (no-qty records
// count as 1 each) and reports the shared verb/object when every record
// agrees, falling back to the section's subject when records name distinct
// verbs or objects.
func summarizeChangeSection(ch ChangesSnapshot) string {
	var total int64
	verb, mixedVerb := ch.Records[0].Verb, false
	object, mixedObject := ch.Records[0].Object, false
	for _, r := range ch.Records {
		if r.HasQty {
			total += r.Quantity
		} else {
			total++
		}
		if r.Verb != verb {
			mixedVerb = true
		}
		if r.Object != object {
			mixedObject = true
		}
	}
	if mixedObject {
		object = ch.Subject
	} else {
		// I4: object arrives singular now (mutation verbs take a singular
		// object); pluralize from the summed quantity here, same as
		// ledgerObject does per-row.
		object = Pluralize(total, object)
	}
	if mixedVerb {
		return fmt.Sprintf("%d %s changed", total, object)
	}
	return fmt.Sprintf("%d %s %s", total, object, verb)
}

func stateColor(s EntityState) string {
	switch s {
	case Done:
		return sgrGreen
	case Failed:
		return sgrRed
	case Blocked:
		return sgrRed
	case Warning:
		return sgrYellow
	case Running:
		return sgrCyan
	case Pending, Skipped, Cancelled, Incomplete, NotStarted:
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
