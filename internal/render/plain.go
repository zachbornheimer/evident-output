package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// compactLayoutMaxWidth switches changes/plans to compact rows.
const compactLayoutMaxWidth = 40

// defaultWidth mirrors the root package's construction default (80 columns,
// option.go) — duplicated as a literal here (not imported) because render
// must never import the root package (see glyph.go's package doc).
const defaultWidth = 80

// Plain projects a snapshot to plain text without terminal ownership.
// width <= 0 falls back to defaultWidth.
func Plain(s core.Snapshot, width int, noColor, verbose bool, profile txt.GlyphProfile) string {
	var b strings.Builder
	if width <= 0 {
		width = defaultWidth
	}
	color := !noColor

	if s.DryRun {
		WriteDryRunMarker(&b, color)
	}

	for _, line := range s.Lines {
		WriteDebugOrLine(&b, line, color)
	}

	for _, t := range s.Tasks {
		WriteTask(&b, t, color, verbose, profile)
	}

	for _, col := range s.Collections {
		WriteCollection(&b, col, color, verbose, profile)
	}

	for _, ch := range s.Changes {
		WriteEffects(&b, "changed", ch.Subject, ch.Records, ch.IntendedVerb, width, color, profile)
	}
	for _, p := range s.Plans {
		WriteEffects(&b, "planned", p.Subject, p.Records, p.IntendedVerb, width, color, profile)
	}

	if s.Conclusion != nil && !ShouldSuppressStandaloneConclusion(s) {
		WriteConclusion(&b, *s.Conclusion, color, profile)
	}

	return b.String()
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

// maxVisibleProblems is the human default bound (OPEN-003). Structured snapshots
// retain full core.Problem lists separately (DEC-FAIL-001/003).
const maxVisibleProblems = 5

// Plain problem indent widths (fixed presentation dialect, not operational knobs).
const (
	// problemTreeIndent prefixes ├─ / └─ / │ problem rows.
	problemTreeIndent = "   "
	// problemDetailIndent continues multi-line Detail under a └─ / │ opener.
	problemDetailIndent = "      "
)

// writeProblem renders one problem row. emphasize is true for a core.Failed/core.Blocked
// task's evidence (release-gate round 6 finding 5): the ├─/│/evidence-glyph
// connectors stay txt.Dim either way — they are decoration — but the evidence
// text itself renders at full intensity so a failure's proof is never the
// lowest-contrast text on screen.
func writeProblem(b *strings.Builder, p core.Problem, color, emphasize bool, profile txt.GlyphProfile) {
	detail, tail := effectiveDetailAndTail(p)
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
		}
		fmt.Fprintf(b, "%s%s %s  %s\n", problemTreeIndent, txt.Dim("├─", color), p.Subject, extra)
		if detail != "" {
			writeProblemDetailLines(b, detail, txt.Dim("│", color), color, emphasize)
		}
		if tail != "" {
			writeProblemDetailLines(b, tail, txt.Dim("│", color), color, emphasize)
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
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, txt.Dim(txt.GlyphEvidence.Render(profile), color), emphasizedText(p.Summary, color, emphasize))
}

// emphasizedText applies txt.Dim unless emphasize opts the text out of demotion —
// the single place that decides "is this text decoration or evidence" for
// the problem-rendering chain.
func emphasizedText(s string, color, emphasize bool) string {
	if emphasize {
		return s
	}
	return txt.Dim(s, color)
}

// effectiveDetailAndTail resolves a core.Problem's Detail and EvidenceTail into
// the pair actually rendered: an explicit Detail always renders (never
// silently discarded by an auto-attached or explicitly requested evidence
// tail), and a distinct EvidenceTail renders as an additional evidence line
// underneath it. When Detail is empty, EvidenceTail alone renders as the
// detail body — DetailTail's original, still-supported shape. An identical
// EvidenceTail (auto-attach filled Detail with the same capture tail a
// caller also passed explicitly via DetailTail) collapses to one line, not a
// duplicate.
func effectiveDetailAndTail(p core.Problem) (detail, tail string) {
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
func writeProblemDetailBlock(b *strings.Builder, summary, detail string, color, emphasize bool, profile txt.GlyphProfile) {
	lines := splitPresentationLines(detail)
	evidence := txt.Dim(txt.GlyphEvidence.Render(profile), color)
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
func writeAction(b *strings.Builder, a core.Action, color bool, profile txt.GlyphProfile) {
	glyph := txt.StyleGlyph(txt.GlyphNextAction.Render(profile), txt.SGRCyan, color)
	if a.Command != nil {
		cmd := a.Command.Executable + " " + strings.Join(a.Command.Args, " ")
		fmt.Fprintf(b, "%s  %s\n", glyph, txt.Style(cmd, txt.SGRCyan, color))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "%s  %s\n", glyph, a.Label)
	}
}

func WriteTask(b *strings.Builder, t core.TaskSnapshot, color, verbose bool, profile txt.GlyphProfile) {
	glyph := txt.StyleGlyph(TaskGlyph(t.State, profile), StateColor(t.State), color)
	label := t.Name
	runningDetail := ""
	if t.State == core.Running {
		runningDetail = runningTaskDetail(t)
	}
	switch {
	case t.Summary != "" && t.State == core.Failed:
		// release-gate round 6 finding 5: a Fail summary is the evidence the
		// reader most needs — it must never render at the lowest contrast on
		// screen. Full intensity here; txt.Dim stays for genuinely subordinate
		// outcomes (core.Done/Skip/Cancel summaries) below.
		//
		// release-gate round 8 finding 4: a task that failed mid-loop still
		// carries the in-flight count (e.g. "1/3") it had when Fail was
		// called — dropping it the instant a task fails would hide exactly
		// the evidence a reader needs most ("how far did it get"). Rendered
		// in the same position a core.Running row shows it (runningTaskDetail).
		if count := progressCountText(t.Progress); count != "" {
			fmt.Fprintf(b, "%s  %s  %s  %s\n", glyph, label, count, t.Summary)
		} else {
			fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, t.Summary)
		}
	case t.Summary != "" && t.State == core.Blocked:
		// release-gate round 6 finding 5: same full-intensity treatment as
		// core.Failed above; core.Blocked never carries in-flight progress (a gate
		// resolves before mutation, never mid-loop), so no count applies.
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, t.Summary)
	case t.Summary != "":
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, txt.Dim(t.Summary, color))
	case runningDetail != "":
		// Default intensity, not txt.Dim: the in-flight phase/progress is the
		// diagnostic signal while work is stalled — txt.Dim is reserved for
		// genuinely subordinate rows (○ pending, - not started, evidence,
		// overflow), per evo-rec.md "Color and txt.Style demotions".
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, label, runningDetail)
	default:
		fmt.Fprintf(b, "%s  %s\n", glyph, label)
	}
	// Problems (including Detail from Capture tails) always follow the row.
	// Early-return on Summary used to drop Fail Detail — a silent dialect hole.
	// emphasize keeps a Fail/Block task's evidence at full intensity (finding 5).
	emphasize := t.State == core.Failed || t.State == core.Blocked
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
		writeProblem(b, core.Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		}, color, emphasize, profile)
	}
	writeTaxonomy(b, "", "skipped", t.Skipped, verbose, color, profile)
	writeTaxonomy(b, "", "kept", t.Kept, verbose, color, profile)
}

// runningTaskDetail composes a core.Running task's plain-mode detail text: its
// progress count (or Bytes fraction), its phase, or both together
// ("14/40  requests") — the durable-line counterpart of writeLiveTaskLine's
// interactive combination, without the live-only bar/heartbeat decoration.
// Returns "" for a core.Running task with neither (never happens through the
// public API, since every path that promotes core.Pending to core.Running sets one).
func runningTaskDetail(t core.TaskSnapshot) string {
	count := progressCountText(t.Progress)
	switch {
	case count != "" && t.Phase != "":
		return count + "  " + t.Phase
	case count != "":
		return count
	default:
		return t.Phase
	}
}

// progressCountText renders p as the fixed "C/T" (Determinate) or
// byte-fraction (BytesKind) count text a core.Running row shows, or "" when p
// carries neither — the one place that decides "does this progress have a
// displayable count," shared by runningTaskDetail (core.Running) and writeTask's
// core.Failed row (release-gate round 8 finding 4) so both projections agree on
// where and how the count reads.
func progressCountText(p core.Progress) string {
	switch {
	case p.Kind == core.BytesKind && p.Total > 0:
		return formatByteProgressFixed(p.Completed, p.Total)
	case p.Kind == core.Determinate && p.Total > 0:
		return fmt.Sprintf("%d/%d", p.Completed, p.Total)
	default:
		return ""
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
func writeTaxonomy(b *strings.Builder, indent, verb string, records []core.TaxonomyRecord, verbose, color bool, profile txt.GlyphProfile) {
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
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
	fmt.Fprintf(b, "%s%s  %s %d  (%s)\n", indent, glyph, verb, len(records), strings.Join(parts, ", "))
	writeTaxonomyCauses(b, indent, records, verbose, color, profile)
	if !verbose {
		return
	}
	for _, reason := range order {
		fmt.Fprintf(b, "%s%s%s: %s\n", indent, problemDetailIndent, reason, txt.TruncateNames(names[reason], 0, profile))
	}
}

// writeTaxonomyCauses renders every records' accumulated Causes as evidence
// under the count row: one bounded └─ line normally (first cause + "(+N
// more)"), the full list under Verbose (one line per cause).
func writeTaxonomyCauses(b *strings.Builder, indent string, records []core.TaxonomyRecord, verbose, color bool, profile txt.GlyphProfile) {
	var causes []string
	for _, r := range records {
		causes = append(causes, r.Causes...)
	}
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

// partitionTaxonomyByReason groups records by reason, preserving first-seen
// order so the rendered partition matches the order reasons were recorded.
func partitionTaxonomyByReason(records []core.TaxonomyRecord) (map[string][]string, []string) {
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
// problem — core.Done included. Evo-rec.md core.Problem 1's final ledger keeps ✓ rows
// like "✓  branches   14 deleted" instead of the parent collapsing to one
// line and erasing the children whose evidence lived only in the live
// region while it was running.
func WriteCollection(b *strings.Builder, col core.TasksSnapshot, color, verbose bool, profile txt.GlyphProfile) {
	glyph := txt.StyleGlyph(TaskGlyph(col.State, profile), StateColor(col.State), color)
	if col.Summary != "" {
		fmt.Fprintf(b, "%s  %s  %s\n", glyph, col.Name, txt.Dim(col.Summary, color))
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
func writeCollectionChild(b *strings.Builder, t core.TaskSnapshot, color, verbose bool, profile txt.GlyphProfile) {
	tg := txt.StyleGlyph(TaskGlyph(t.State, profile), StateColor(t.State), color)
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
	emphasize := t.State == core.Failed || t.State == core.Blocked
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
		writeProblem(b, core.Problem{
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
// render time via txt.Pluralize, instead of every call site hand-composing its
// own singular/plural noun with evo.Pluralize. txt.Pluralize itself stays
// exported for prose outside the ledger (a Printf line, a core.Problem detail).
func ledgerObject(r core.EffectRecord) string {
	if !r.HasQty {
		return r.Object
	}
	return txt.Pluralize(r.Quantity, r.Object)
}

// mergeIdenticalEffectRecords combines repeated records that share the same
// (verb, object) pair — one ledger's tense is fixed for every record it
// holds, so verb+object alone identifies a duplicate row — into one row with
// summed quantities: twelve `Delete(1, "merged branch")` calls render
// "deleted 12 merged branches", not twelve identical rows plus an overflow
// ellipsis (release-gate finding 6). Distinct records (different verb or
// object) are left alone and keep their original relative order. The model
// itself is untouched — only this presentation-layer view merges.
func mergeIdenticalEffectRecords(records []core.EffectRecord) []core.EffectRecord {
	type key struct{ verb, object string }
	index := make(map[key]int, len(records))
	merged := make([]core.EffectRecord, 0, len(records))
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

func WriteEffects(b *strings.Builder, kind, subject string, records []core.EffectRecord, intendedVerb string, width int, color bool, profile txt.GlyphProfile) {
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
	tag := txt.Style(fmt.Sprintf("[%s]", kind), effectColor(kind), color)
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
		verb := txt.PadRight(r.Verb, maxVerb)
		if r.HasQty {
			qty := txt.PadLeft(strconv.FormatInt(r.Quantity, 10), maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qty, ledgerObject(r))
			continue
		}
		gap := maxVerb - len(r.Verb)
		if gap > maxLeader {
			gap = maxLeader
		}
		if gap > 2 {
			leader := strings.Repeat("·", gap)
			fmt.Fprintf(b, "  %s%s %s\n", r.Verb, txt.Dim(leader, color), r.Object)
		} else {
			qtyPad := txt.PadLeft("", maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qtyPad, r.Object)
		}
	}
	writeEffectOverflow(b, omitted, color, profile)
}

// writeEffectOverflow renders the bounded-rows omission line. The overflow
// glyph (txt.Dim "…"/"...") marks it, not "!" — an omitted-count line is a
// viewport limit, not something demanding attention (evo-rec.md "! is
// attention only... Overflow is never !").
func writeEffectOverflow(b *strings.Builder, omitted int, color bool, profile txt.GlyphProfile) {
	if omitted <= 0 {
		return
	}
	fmt.Fprintf(b, "  %s  +%d more (not shown)\n", txt.Dim(txt.GlyphOverflow.Render(profile), color), omitted)
}

// dryRunMarkerText is the fixed announcement body for a dry-run run's
// opening line (evo-rec.md core.Problem 1: "a dry run must announce itself").
const dryRunMarkerText = "no changes will be made"

// writeDryRunMarker emits the unmissable dry-run marker line. It renders
// once, first, styled like [planned] — a caller cannot opt out or bury it,
// because every dry-run projection (RenderPlain and Finish's residual) calls
// this before any other row.
func WriteDryRunMarker(b *strings.Builder, color bool) {
	tag := txt.Style("[dry-run]", effectColor("planned"), color)
	fmt.Fprintf(b, "%s  %s\n", tag, dryRunMarkerText)
}

// conclusionPartialModifier is the literal suffix that marks the printed
// band as evo-rec.md's completeness axis rather than a new headline: a run
// that never invented a State of its own (StatePartial is dead precisely
// because Partial is a modifier, not a root verdict) still needs an honest
// band when core.Conclusion.Partial is true (release-gate round 4 finding 1) — an
// abandoned Each loop or a forgotten terminal verb on an otherwise clean
// finish must not read as silently complete.
const conclusionPartialModifier = " · partial"

// conclusionWarnedModifier marks the printed band with the same "modifier,
// not a new headline" treatment as conclusionPartialModifier (release-gate
// round 8 finding 3): a run that resolved at least one core.Warning while its
// headline settled on an OK-family state (e.g. [ready]) must not read as
// silently clean — the exit code is unchanged, only the band gains this
// suffix. core.Conclusion.Warned is already false when State is itself
// core.StateWarning, so the two never double up.
const conclusionWarnedModifier = " · warned"

// conclusionBandTag renders the trailing outcome band's bracketed tag,
// appending conclusionPartialModifier and/or conclusionWarnedModifier when
// the conclusion carries either — evo-rec.md's spec does not pin a literal
// spelling for these modifier bands (only the two/three-axis rule), so the
// modifier form here is the documented implementation choice (see work
// order for the corresponding spec-edit note).
func conclusionBandTag(c core.Conclusion) string {
	tag := string(c.State)
	if c.Partial {
		tag += conclusionPartialModifier
	}
	if c.Warned {
		tag += conclusionWarnedModifier
	}
	return fmt.Sprintf("[%s]", tag)
}

func WriteConclusion(b *strings.Builder, c core.Conclusion, color bool, profile txt.GlyphProfile) {
	tag := txt.Style(conclusionBandTag(c), conclusionColor(c.State), color)
	// A bare Subject that equals the headline state word itself ("changed",
	// "failed", ...) says nothing the bracketed tag hasn't already said — it
	// is what an unconfigured Config.Title falls back to, not a caller's
	// chosen subject, so printing it stutters the band ("[changed]  changed",
	// release-gate round 10 finding 1). Suppress it instead of repeating it.
	if c.Subject != "" && c.Subject != string(c.State) {
		fmt.Fprintf(b, "\n%s  %s\n", tag, txt.Style(c.Subject, txt.SGRBold, color))
	} else {
		fmt.Fprintf(b, "\n%s\n", tag)
	}
	if c.Explanation != "" {
		fmt.Fprintf(b, "  %s\n", c.Explanation)
	}
	if c.State == core.StateCancelled || c.State == core.StateFailed {
		writeAlreadyMutated(b, c.Changes, color, profile)
	}
	for _, a := range c.Actions {
		writeAction(b, a, color, profile)
	}
}

// writeAlreadyMutated renders the early-termination "! already mutated: ..."
// line. It fires whenever a run concludes core.Cancelled or core.Failed with at least
// one committed effect — "!" is attention-only (evo-rec.md "Tightened glyph
// vocabulary"), and an empty ledger earns no attention, so the row is
// suppressed entirely rather than rendered as "none". The summary is derived
// mechanically from the Changes ledger, never assembled by the caller
// (evo-rec.md "Taxonomy and mutation lines are derived, never assembled").
func writeAlreadyMutated(b *strings.Builder, changes []core.ChangesSnapshot, color bool, profile txt.GlyphProfile) {
	summary, ok := summarizeAlreadyMutated(changes)
	if !ok {
		return
	}
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
	fmt.Fprintf(b, "%s  already mutated: %s\n", glyph, summary)
}

// summarizeAlreadyMutated derives one compact fragment per non-empty Changes
// section (e.g. "8 branches deleted"), joined with "; ". ok is false when no
// section committed any effect, telling the caller to suppress the row.
func summarizeAlreadyMutated(changes []core.ChangesSnapshot) (string, bool) {
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
func summarizeChangeSection(ch core.ChangesSnapshot) string {
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
		object = txt.Pluralize(total, object)
	}
	if mixedVerb {
		return fmt.Sprintf("%d %s changed", total, object)
	}
	return fmt.Sprintf("%d %s %s", total, object, verb)
}

func StateColor(s core.EntityState) string {
	switch s {
	case core.Done:
		return txt.SGRGreen
	case core.Failed:
		return txt.SGRRed
	case core.Blocked:
		return txt.SGRRed
	case core.Warning:
		return txt.SGRYellow
	case core.Running:
		return txt.SGRCyan
	case core.Pending, core.Skipped, core.Cancelled, core.Incomplete, core.NotStarted:
		return txt.SGRDim
	default:
		return ""
	}
}

func conclusionColor(s core.ConclusionState) string {
	switch s {
	case core.StateReady, core.StateUnchanged, core.StateChanged:
		return txt.SGRGreen
	case core.StatePlanned:
		return txt.SGRBlue
	case core.StateFailed:
		return txt.SGRRed
	case core.StateBlocked:
		return txt.SGRRed
	case core.StateCancelled:
		return txt.SGRDim
	default:
		return txt.SGRCyan
	}
}

func effectColor(kind string) string {
	switch kind {
	case "changed":
		return txt.SGRGreen
	case "planned":
		return txt.SGRBlue
	default:
		return txt.SGRCyan
	}
}
