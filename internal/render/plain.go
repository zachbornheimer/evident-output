package render

import (
	"fmt"
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
	s = HumanProjection(s, verbose)

	if s.DryRun {
		WritePlannedHeader(&b, color, s.Preview, s.DryRunSubject)
	}

	for _, line := range s.Lines {
		WriteDebugOrLine(&b, line, color)
	}
	writeRunAnnotations(&b, s.Warnings, s.Facts, color, profile)

	taskNameWidth := maxTaskNameWidth(s.Tasks)
	for _, t := range s.Tasks {
		WriteTaskAligned(&b, t, taskNameWidth, color, verbose, profile)
	}

	for _, col := range s.Collections {
		WriteCollection(&b, col, color, verbose, profile)
	}

	if hasTaskRows(s) && hasEffectSections(s) {
		b.WriteByte('\n')
	}

	changeNameWidth := maxEffectSubjectWidth(s.Changes, func(c core.ChangesSnapshot) string { return c.Subject })
	for _, ch := range s.Changes {
		WriteEffects(&b, "changed", ch.Subject, changeNameWidth, ch.Records, ch.IntendedVerb, width, color, profile)
	}
	planNameWidth := maxEffectSubjectWidth(s.Plans, func(p core.PlanSnapshot) string { return p.Subject })
	for _, p := range s.Plans {
		WriteEffects(&b, "planned", p.Subject, planNameWidth, p.Records, p.IntendedVerb, width, color, profile)
	}

	if s.Conclusion != nil && !ShouldSuppressStandaloneConclusion(s) {
		WriteConclusion(&b, StandaloneConclusion(s), color, profile)
	}

	return b.String()
}

// hasTaskRows reports whether s rendered any task or collection rows above
// the effects ledger — the blank-line separator below only belongs between
// two real blocks, never floating above an empty task section.
func hasTaskRows(s core.Snapshot) bool {
	return len(s.Tasks) > 0 || len(s.Collections) > 0
}

// hasEffectSections reports whether s has a [changed]/[planned] ledger to
// render — the blank line separating it from the task block above
// (fixture-repo-retire-dryrun.md: a blank line sits between the last task
// row and the first ledger row) only belongs when both sides are non-empty.
func hasEffectSections(s core.Snapshot) bool {
	return len(s.Changes) > 0 || len(s.Plans) > 0
}

// taskNameColumnMargin is the one extra column an inline annotation's own
// text column carries beyond the widest sibling name (fixture-repo-retire-
// dryrun.md, measured byte-for-byte: the widest name — "remote-tracking",
// 15 cells — still leaves one blank column of its own before the fixed
// 2-space gap to its "! "-prefixed or ledger-verb text, not zero). It
// applies only where that fixture pins it — an inline warning/fact/taxonomy
// annotation and a WriteEffects ledger row — never to a caller-supplied
// Summary, which keeps the plain nameWidth+2 gap evo-rec.md's own examples
// pin (TestSpecP16 etc.): the two annotation kinds render through different
// visual conventions ("! text" / bare verb vs. a caller's own words) and are
// measured against different fixtures.
const taskNameColumnMargin = 1

// maxTaskNameWidth returns the shared column sibling root tasks pad their
// name to (fixture-repo-retire-dryrun.md) — the widest name's cell width. A
// lone task (or none) needs no alignment, so callers pass the result
// straight to WriteTaskAligned's nameWidth, where 0 means "don't pad".
func maxTaskNameWidth(tasks []core.TaskSnapshot) int {
	if len(tasks) < 2 {
		return 0
	}
	width := 0
	for _, t := range tasks {
		if n := len([]rune(t.Name)); n > width {
			width = n
		}
	}
	return width
}

// maxEffectSubjectWidth returns the shared column WriteEffects' one-
// line-per-subject form pads its subject to, before taskNameColumnMargin and
// the verb gap (fixture-repo-retire-dryrun.md's "[planned] branches   delete
// ..." / "[planned] worktrees  remove ..."). subjectOf extracts the
// comparable field since ChangesSnapshot and PlanSnapshot are distinct types
// with no shared interface.
func maxEffectSubjectWidth[T any](sections []T, subjectOf func(T) string) int {
	if len(sections) < 2 {
		return 0
	}
	width := 0
	for _, s := range sections {
		if n := txt.Cells(subjectOf(s)); n > width {
			width = n
		}
	}
	return width
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
	// taskAnnotationIndent nests a standalone task's annotations — taxonomy
	// tallies, verification details, warnings, facts — under its row
	// (spec §26/§27: "✓ branches  50 checked" / "  ! kept 13 (...)").
	taskAnnotationIndent = "  "
	// groupChildIndent nests a Group header's children: its child rows and
	// the tallies its folded items leave behind, in one column.
	groupChildIndent = "   "
)

// writeVerificationDetails renders a Task's per-attribute reconciliation
// outcomes (spec §2, §8.2, §20-21, §41, §49) — evo.File/evo.Exec's third
// evidence layer, which subconditions were satisfied or failed, not just
// that the operation as a whole did. A satisfied attribute is a muted "-
// <name>  already satisfied" line; a failed one keeps its own glyph and
// nests whatever Facts explain it one level deeper. Every attribute
// renders once the task itself failed — an isolated failure must never
// look like every attribute is suspect — and, mirroring Fact/Warning's own
// visibility rule, the same full list renders under Verbose even when the
// task succeeded; a clean run says nothing extra by default. indent is the
// row's own nesting indent (matches the value writeNestedTaskFacts/
// writeNestedTaskWarnings already use at this call site — "  " for a
// standalone task, problemTreeIndent for a collection child).
func writeVerificationDetails(b *strings.Builder, details []core.VerificationDetail, indent string, taskFailed, verbose, color bool, profile txt.GlyphProfile) {
	if len(details) == 0 || (!taskFailed && !verbose) {
		return
	}
	for _, d := range details {
		if d.Status == core.VerificationSatisfied {
			glyph := txt.StyleGlyph(TaskGlyph(core.NotStarted, profile), StateColor(core.NotStarted), color)
			fmt.Fprintf(b, "%s%s %s\n", indent, glyph, txt.Dim(d.Name+"  already satisfied", color))
			continue
		}
		glyph := txt.StyleGlyph(TaskGlyph(core.Failed, profile), StateColor(core.Failed), color)
		fmt.Fprintf(b, "%s%s %s\n", indent, glyph, d.Name)
		writeVerificationFacts(b, d.Facts, indent+"  ")
	}
}

// writeVerificationFacts renders one failed VerificationDetail's own Facts
// (error, path, mode, ...) one level deeper than its "✗ <name>" row, each
// name padded to the widest name in this detail alone so the values line
// up in one column (the same PadRight alignment convention taxonomy/effect
// columns already use).
func writeVerificationFacts(b *strings.Builder, facts []core.Fact, indent string) {
	width := 0
	for _, f := range facts {
		if w := txt.Cells(f.Name); w > width {
			width = w
		}
	}
	for _, f := range facts {
		fmt.Fprintf(b, "%s%s  %s\n", indent, txt.PadRight(f.Name, width), f.Value)
	}
}

// warningInlineMaxCells bounds how many display cells a Done task's one
// warning may occupy before it must move off the ✓ row onto its own nested
// "!" line (P2) — the same threshold evo-rec.md's compact layout already
// uses for row width, measured in display cells (E2.5 finding 6): a plain
// byte-length check overcounts multi-byte runes and undercounts wide ones,
// so it agrees with the compact-layout width check only by coincidence.
const warningInlineMaxCells = compactLayoutMaxWidth

// bangColumnFiller is as wide as inlineWarningText's "! " glyph+space
// prefix — a Fact's inline text stands in this much blank space so its own
// text starts at the same column a sibling row's "! <text>" would
// (fixture-repo-retire-dryrun.md, measured byte-for-byte: "remote-tracking"
// carries a bare Fact while its siblings carry "! kept ..."; all three
// annotation texts land in one column).
const bangColumnFiller = "  "

// WriteTask renders a standalone task row unpadded — see WriteTaskAligned
// for the sibling-column-alignment form fixture-repo-retire-dryrun.md
// requires when annotations (inline warnings/facts) sit among peers.
func WriteTask(b *strings.Builder, t core.TaskSnapshot, color, verbose bool, profile txt.GlyphProfile) {
	WriteTaskAligned(b, t, 0, color, verbose, profile)
}

// WriteTaskAligned renders a task row with its name padded to nameWidth (0
// = no padding) before any annotation, so a run of sibling tasks with inline
// warnings/facts line up in one column ("✓ branches          ! kept 13...",
// fixture-repo-retire-dryrun.md) instead of each row's detail starting
// wherever its own name happens to end. A detail-less row's padding is
// trimmed so it never ends in dangling whitespace (mirrors DisplayUnit.Render).
func WriteTaskAligned(b *strings.Builder, t core.TaskSnapshot, nameWidth int, color, verbose bool, profile txt.GlyphProfile) {
	t = TaskAtVerbosity(t, verbose)
	glyph := txt.StyleGlyph(TaskGlyph(t.State, profile), StateColor(t.State), color)
	label := txt.PadRight(t.Name, nameWidth)
	// annotatedLabel carries taskNameColumnMargin's extra column — only the
	// inline warning/fact/taxonomy cases below use it, never a Summary row.
	annotatedLabel := txt.PadRight(t.Name, nameWidth+taskNameColumnMargin)
	runningDetail := ""
	if t.State == core.Running {
		runningDetail = runningTaskDetail(t)
	}
	inlineWarning, hasInlineWarning := inlineTaskWarning(t)
	nestedWarnings := t.Warnings
	if hasInlineWarning {
		nestedWarnings = nil
	}
	inlineFact, hasInlineFact := inlineTaskFact(t)
	nestedFacts := t.Facts
	if hasInlineFact {
		nestedFacts = nil
	}
	inlineTaxonomy, inlineTaxonomyVerb, hasInlineTaxonomy := inlineTaskTaxonomy(t)
	switch {
	case t.Resolution == core.ResolutionAlreadySatisfied:
		fmt.Fprintf(b, "%s %s  %s\n", glyph, label, alreadySatisfiedRowDetail(t, color))
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
			fmt.Fprintf(b, "%s %s  %s  %s\n", glyph, label, count, t.Summary)
		} else {
			fmt.Fprintf(b, "%s %s  %s\n", glyph, label, t.Summary)
		}
	case t.Summary != "" && t.State == core.Blocked:
		// release-gate round 6 finding 5: same full-intensity treatment as
		// core.Failed above; core.Blocked never carries in-flight progress (a gate
		// resolves before mutation, never mid-loop), so no count applies.
		fmt.Fprintf(b, "%s %s  %s\n", glyph, label, t.Summary)
	case t.Summary != "":
		fmt.Fprintf(b, "%s %s  %s\n", glyph, label, txt.Dim(t.Summary, color))
	case hasInlineWarning:
		fmt.Fprintf(b, "%s %s  %s\n", glyph, annotatedLabel, inlineWarningText(inlineWarning, color, profile))
	case hasInlineTaxonomy:
		fmt.Fprintf(b, "%s %s  %s\n", glyph, annotatedLabel, inlineTaxonomyText(inlineTaxonomy, inlineTaxonomyVerb, color, profile))
	case hasInlineFact:
		fmt.Fprintf(b, "%s %s  %s\n", glyph, annotatedLabel, inlineFactText(inlineFact, color))
	case runningDetail != "":
		// Default intensity, not txt.Dim: the in-flight phase/progress is the
		// diagnostic signal while work is stalled — txt.Dim is reserved for
		// genuinely subordinate rows (○ pending, - not started, evidence,
		// overflow), per evo-rec.md "Color and txt.Style demotions".
		fmt.Fprintf(b, "%s %s  %s\n", glyph, label, runningDetail)
	default:
		// No detail to align a column against — pad-trimmed so this row
		// never ends in dangling whitespace (DisplayUnit.Render's rule).
		fmt.Fprintf(b, "%s %s\n", glyph, strings.TrimRight(label, " "))
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
		p = dedupeEvidenceTailAgainstRow(p, t.Summary)
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
	writeDispositions(b, taskAnnotationIndent, taskDispositions(t), inlineTaxonomyVerb, verbose, color, profile)
	writeVerificationDetails(b, t.Verification, taskAnnotationIndent, t.State == core.Failed, verbose, color, profile)
	writeNestedTaskWarnings(b, nestedWarnings, taskAnnotationIndent, color, profile)
	writeNestedTaskFacts(b, nestedFacts, taskAnnotationIndent, color)
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

// writeCollection renders a Tasks group: the parent glyph/name (with its own
// Summary when set), then every resolved child row with its own summary or
// problem — core.Done included. Evo-rec.md core.Problem 1's final ledger keeps ✓ rows
// like "✓  branches   14 deleted" instead of the parent collapsing to one
// line and erasing the children whose evidence lived only in the live
// region while it was running.
func WriteCollection(b *strings.Builder, col core.TasksSnapshot, color, verbose bool, profile txt.GlyphProfile) {
	writeCollectionAligned(b, col, 0, color, verbose, profile)
}

// writeCollectionAligned is WriteCollection with the name column a
// collapsed one-row collection pads to (0 = its own name), so a header-less
// parent's rows line up (headerlessRowNameWidth).
func writeCollectionAligned(b *strings.Builder, col core.TasksSnapshot, nameWidth int, color, verbose bool, profile txt.GlyphProfile) {
	col, items := withoutDispositionItems(col)
	if rendersAsOwnTask(col) {
		WriteTaskAligned(b, col.Tasks[0], nameWidth, color, verbose, profile)
		writeDispositions(b, taskAnnotationIndent, items, noDisposition, verbose, color, profile)
		return
	}
	if groupHeaderAddsNothing(col) && items.Empty() {
		writeHeaderlessGroup(b, col, color, verbose, profile)
		return
	}
	glyph := txt.StyleGlyph(TaskGlyph(col.State, profile), StateColor(col.State), color)
	if col.Summary != "" {
		fmt.Fprintf(b, "%s %s  %s\n", glyph, col.Name, txt.Dim(col.Summary, color))
	} else {
		fmt.Fprintf(b, "%s %s\n", glyph, col.Name)
	}
	writeDispositions(b, headerTallyIndent(col), items, noDisposition, verbose, color, profile)
	childNameWidth := maxTaskNameWidth(col.Tasks)
	for _, t := range col.Tasks {
		writeCollectionChild(b, t, childNameWidth, color, verbose, profile)
	}
	// Nested containers (P3's recursive .Sequence/.Group nesting)
	// render as an indented sub-group, one level per nesting depth.
	for _, child := range col.Collections {
		var nested strings.Builder
		WriteCollection(&nested, child, color, verbose, profile)
		for line := range strings.SplitSeq(strings.TrimRight(nested.String(), "\n"), "\n") {
			fmt.Fprintf(b, "%s%s\n", groupChildIndent, line)
		}
	}
}

// writeCollectionChild renders one child task row under its parent group:
// glyph, name, and whichever of problem summary / task summary explains it,
// then every problem's Detail/evidence line the same way a standalone
// writeTask already does — a collection child is a task, and dropping its
// evidence (└─ ...) and taxonomy here was the gap that forced the
// repo-retire adoption off the Group/Tasks API.
func writeCollectionChild(b *strings.Builder, t core.TaskSnapshot, nameWidth int, color, verbose bool, profile txt.GlyphProfile) {
	t = TaskAtVerbosity(t, verbose)
	tg := txt.StyleGlyph(TaskGlyph(t.State, profile), StateColor(t.State), color)
	name := txt.PadRight(t.Name, nameWidth)
	// annotatedName carries taskNameColumnMargin's extra column — only the
	// inline warning/fact/taxonomy cases below use it, never a Summary row
	// (mirrors WriteTaskAligned's annotatedLabel; see taskNameColumnMargin).
	annotatedName := txt.PadRight(t.Name, nameWidth+taskNameColumnMargin)
	headerSummary := t.Summary
	inlineWarning, hasInlineWarning := inlineTaskWarning(t)
	nestedWarnings := t.Warnings
	inlineFact, hasInlineFact := inlineTaskFact(t)
	nestedFacts := t.Facts
	inlineTaxonomy, inlineTaxonomyVerb, hasInlineTaxonomy := inlineTaskTaxonomy(t)
	var row strings.Builder
	hasDetail := true
	switch {
	case len(t.Problems) > 0:
		headerSummary = t.Problems[0].Summary
		fmt.Fprintf(&row, "   %s %s  %s", tg, name, headerSummary)
	case t.Resolution == core.ResolutionAlreadySatisfied:
		headerSummary = alreadySatisfiedDetail
		fmt.Fprintf(&row, "   %s %s  %s", tg, name, alreadySatisfiedRowDetail(t, color))
	case headerSummary != "":
		fmt.Fprintf(&row, "   %s %s  %s", tg, name, headerSummary)
	case hasInlineWarning:
		fmt.Fprintf(&row, "   %s %s  %s", tg, annotatedName, inlineWarningText(inlineWarning, color, profile))
		nestedWarnings = nil
	case hasInlineTaxonomy:
		fmt.Fprintf(&row, "   %s %s  %s", tg, annotatedName, inlineTaxonomyText(inlineTaxonomy, inlineTaxonomyVerb, color, profile))
	case hasInlineFact:
		fmt.Fprintf(&row, "   %s %s  %s", tg, annotatedName, inlineFactText(inlineFact, color))
		nestedFacts = nil
	default:
		fmt.Fprintf(&row, "   %s %s", tg, name)
		hasDetail = false
	}
	if hasDetail {
		b.WriteString(row.String())
	} else {
		// No detail to align a column against — the padded name's trailing
		// spaces would otherwise dangle at line end (DisplayUnit.Render's rule).
		b.WriteString(strings.TrimRight(row.String(), " "))
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
		p = dedupeEvidenceTailAgainstRow(p, headerSummary)
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
	writeDispositions(b, problemTreeIndent, taskDispositions(t), inlineTaxonomyVerb, verbose, color, profile)
	writeVerificationDetails(b, t.Verification, problemTreeIndent, t.State == core.Failed, verbose, color, profile)
	writeNestedTaskWarnings(b, nestedWarnings, problemTreeIndent, color, profile)
	writeNestedTaskFacts(b, nestedFacts, problemTreeIndent, color)
}

// maxVisibleEffectRows bounds how many plan/changes rows the human view
// renders per section (evo-rec.md "bound visible named rows... model
// keeps all records"). The snapshot always retains the full record list;
// only this presentation loop is capped. Mirrors maxVisibleProblems's bound.
const maxVisibleEffectRows = maxVisibleProblems

// dryRunMarkerText is the fixed announcement body for a dry-run run's
// opening line (evo-rec.md core.Problem 1: "a dry run must announce itself").
const dryRunMarkerText = "no changes will be made"

// conclusionPartialModifier is the literal suffix that marks the printed
// band as evo-rec.md's completeness axis rather than a new headline: a run
// that never invented a State of its own (StatePartial is dead precisely
// because Partial is a modifier, not a root verdict) still needs an honest
// band when core.Conclusion.Partial is true (release-gate round 4 finding 1) — an
// abandoned per-item loop or a forgotten terminal verb on an otherwise clean
// finish must not read as silently complete.
const conclusionPartialModifier = " · partial"

// conclusionWarnedModifier marks the printed band with the same "modifier,
// not a new headline" treatment as conclusionPartialModifier (release-gate
// round 8 finding 3): a run that carries at least one TaskHandle.Warn
// annotation (P2: Warn never resolves its own lifecycle state) while its
// headline settled on an OK-family state (e.g. [ready]) must not read as
// silently clean — the exit code is unchanged, only the band gains this
// suffix. core.Conclusion.Warned is already false when State is itself
// core.StateWarning, so the two never double up.
const conclusionWarnedModifier = " · warned"
