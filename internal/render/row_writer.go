package render

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// DefaultWidth mirrors the root package's construction default (80 columns,
// option.go) — duplicated as a literal here (not imported) because render
// must never import the root package (see glyph.go's package doc).
const DefaultWidth = 80

// Plain projects a snapshot to plain text without terminal ownership.
// width <= 0 falls back to DefaultWidth.
func Plain(s core.Snapshot, width int, noColor, verbose bool, profile txt.GlyphProfile) string {
	var b strings.Builder
	if width <= 0 {
		width = DefaultWidth
	}
	st := Style{Color: !noColor, Verbose: verbose, Profile: profile}
	s = HumanProjection(s, verbose)

	if s.DryRun {
		WritePlannedHeader(&b, st.Color, s.Preview, s.DryRunSubject)
	}

	for _, line := range s.Lines {
		WriteDebugOrLine(&b, line, st.Color)
	}
	writeRunAnnotations(&b, s.Warnings, s.Facts, st)

	taskNameWidth := maxTaskNameWidth(s.Tasks)
	for _, t := range s.Tasks {
		WriteTaskAligned(&b, t, taskNameWidth, st)
	}

	for _, col := range s.Collections {
		WriteCollection(&b, col, st)
	}

	if HasTaskRows(s) && HasEffectSections(s) {
		b.WriteByte('\n')
	}

	WriteLedger(&b, s, width, st)

	if s.Conclusion != nil && !ShouldSuppressStandaloneConclusion(s) {
		WriteConclusion(&b, StandaloneConclusion(s), st)
	}

	return b.String()
}

// HasTaskRows reports whether s rendered any task or collection rows above
// the effects ledger — the blank-line separator below only belongs between
// two real blocks, never floating above an empty task section.
func HasTaskRows(s core.Snapshot) bool {
	return len(s.Tasks) > 0 || len(s.Collections) > 0
}

// HasEffectSections reports whether s has a [changed]/[planned] ledger to
// render — the blank line separating it from the task block above
// (fixture-repo-retire-dryrun.md: a blank line sits between the last task
// row and the first ledger row) only belongs when both sides are non-empty.
func HasEffectSections(s core.Snapshot) bool {
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
)

// WriteVerificationDetails renders a Task's per-attribute reconciliation
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
// WriteNestedTaskWarnings already use at this call site — "  " for a
// standalone task, problemTreeIndent for a collection child).
func WriteVerificationDetails(b *strings.Builder, details []core.VerificationDetail, indent string, taskFailed bool, s Style) {
	if len(details) == 0 || (!taskFailed && !s.Verbose) {
		return
	}
	for _, d := range details {
		if d.Status == core.VerificationSatisfied {
			fmt.Fprintf(b, "%s%s %s\n", indent, s.StateGlyph(core.NotStarted), s.Dim(d.Name+"  already satisfied"))
			continue
		}
		fmt.Fprintf(b, "%s%s %s\n", indent, s.StateGlyph(core.Failed), d.Name)
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
const warningInlineMaxCells = CompactLayoutMaxWidth

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
func WriteTask(b *strings.Builder, t core.TaskSnapshot, s Style) {
	WriteTaskAligned(b, t, 0, s)
}

// WriteTaskAligned renders a root task row with its name padded to
// nameWidth (0 = no padding) before any annotation, so a run of sibling
// tasks with inline warnings/facts line up in one column ("✓ branches
// ! kept 13...", fixture-repo-retire-dryrun.md). See taskRow.
func WriteTaskAligned(b *strings.Builder, t core.TaskSnapshot, nameWidth int, s Style) {
	rootRow(t, nameWidth).write(b, s)
}

// runningTaskDetail composes a core.Running task's plain-mode detail text: its
// progress count (or Bytes fraction), its phase, or both together
// ("14/40  requests") — the durable-line counterpart of writeLiveTaskLine's
// interactive combination, without the live-only bar/heartbeat decoration.
// Returns "" for a core.Running task with neither (never happens through the
// public API, since every path that promotes core.Pending to core.Running sets one).
func runningTaskDetail(t core.TaskSnapshot) string {
	count := ProgressCountText(t.Progress)
	switch {
	case count != "" && t.Phase != "":
		return count + "  " + t.Phase
	case count != "":
		return count
	default:
		return t.Phase
	}
}

// ProgressCountText renders p as the fixed "C/T" (Determinate) or
// byte-fraction (BytesKind) count text a core.Running row shows, or "" when p
// carries neither — the one place that decides "does this progress have a
// displayable count," shared by runningTaskDetail (core.Running) and writeTask's
// core.Failed row (release-gate round 8 finding 4) so both projections agree on
// where and how the count reads.
func ProgressCountText(p core.Progress) string {
	switch {
	case p.Kind == core.BytesKind && p.Total > 0:
		return FormatByteProgressFixed(p.Completed, p.Total)
	case p.Kind == core.Determinate && p.Total > 0:
		return fmt.Sprintf("%d/%d", p.Completed, p.Total)
	default:
		return ""
	}
}

// WriteCollection renders a Tasks group: the parent glyph/name (with its own
// Summary when set), then every resolved child row with its own summary or
// problem — core.Done included. Evo-rec.md core.Problem 1's final ledger keeps ✓ rows
// like "✓  branches   14 deleted" instead of the parent collapsing to one
// line and erasing the children whose evidence lived only in the live
// region while it was running.
func WriteCollection(b *strings.Builder, col core.TasksSnapshot, s Style) {
	writeCollectionAligned(b, col, 0, s)
}

// writeCollectionAligned is WriteCollection with the name column a
// collapsed one-row collection pads to (0 = its own name), so a header-less
// parent's rows line up (HeaderlessRowNameWidth).
func writeCollectionAligned(b *strings.Builder, col core.TasksSnapshot, nameWidth int, s Style) {
	col, items := WithoutDispositionItems(col)
	switch {
	case RendersAsOwnTask(col):
		WriteTaskAligned(b, col.Tasks[0], nameWidth, s)
		WriteDispositions(b, TaskAnnotationIndent, items, NoDisposition, s)
	case FlattensHeader(col, items):
		writeHeaderlessGroup(b, col, s)
	default:
		writeCollectionHeader(b, col, s)
		WriteDispositions(b, HeaderTallyIndent(col), items, NoDisposition, s)
		writeCollectionBody(b, col, s)
	}
}

// writeCollectionHeader writes a Group or Sequence's own row.
func writeCollectionHeader(b *strings.Builder, col core.TasksSnapshot, s Style) {
	unit := DisplayUnit{Glyph: s.StateGlyph(col.State), Name: col.Name}
	if col.Summary != "" {
		unit.Detail = s.Dim(col.Summary)
	}
	b.WriteString(unit.Render(""))
	b.WriteByte('\n')
}

// writeCollectionBody writes a headed container's child rows, then its
// nested containers indented one level per nesting depth (P3).
func writeCollectionBody(b *strings.Builder, col core.TasksSnapshot, s Style) {
	childNameWidth := maxTaskNameWidth(col.Tasks)
	for _, t := range col.Tasks {
		childRow(t, childNameWidth).write(b, s)
	}
	for _, child := range col.Collections {
		var nested strings.Builder
		WriteCollection(&nested, child, s)
		for line := range strings.SplitSeq(strings.TrimRight(nested.String(), "\n"), "\n") {
			fmt.Fprintf(b, "%s%s\n", GroupChildIndent, line)
		}
	}
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
// band as evo-rec.md's completeness axis rather than a new Headline: a run
// that never invented a State of its own (StatePartial is dead precisely
// because Partial is a modifier, not a root verdict) still needs an honest
// band when core.Conclusion.Partial is true (release-gate round 4 finding 1) — an
// abandoned per-item loop or a forgotten terminal verb on an otherwise clean
// finish must not read as silently complete.
const conclusionPartialModifier = " · partial"

// conclusionWarnedModifier marks the printed band with the same "modifier,
// not a new Headline" treatment as conclusionPartialModifier (release-gate
// round 8 finding 3): a run that carries at least one warning-severity
// Problem annotation (P2: a warning-severity Problem never resolves its
// own lifecycle state) while its
// Headline settled on an OK-family state (e.g. [ready]) must not read as
// silently clean — the exit code is unchanged, only the band gains this
// suffix. core.Conclusion.Warned is already false when State is itself
// core.StateWarning, so the two never double up.
const conclusionWarnedModifier = " · warned"

// writeHeaderlessGroup renders a Group's children as siblings of the
// surrounding rows, aligned to one name column, then any nested containers
// the same way.
func writeHeaderlessGroup(b *strings.Builder, col core.TasksSnapshot, s Style) {
	nameWidth := HeaderlessRowNameWidth(col)
	for _, t := range col.Tasks {
		WriteTaskAligned(b, t, nameWidth, s)
	}
	for _, child := range col.Collections {
		writeCollectionAligned(b, child, nameWidth, s)
	}
}
