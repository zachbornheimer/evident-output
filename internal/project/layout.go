package project

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// CompactLayoutMaxWidth switches changes/plans to compact rows.
const CompactLayoutMaxWidth = 40

// DefaultWidth mirrors the root package's construction default (80 columns,
// option.go) — duplicated as a literal here (not imported) because render
// must never import the root package (see glyph.go's package doc).
const DefaultWidth = 80

// HasTaskRows reports whether s rendered any task or collection rows above
// the effects ledger — the blank-line separator below only belongs between
// two real blocks, never floating above an empty task section.
func HasTaskRows(s record.Snapshot) bool {
	return len(s.Tasks) > 0 || len(s.Collections) > 0
}

// HasEffectSections reports whether s has a [changed]/[planned] ledger to
// render — the blank line separating it from the task block above
// (fixture-repo-retire-dryrun.md: a blank line sits between the last task
// row and the first ledger row) only belongs when both sides are non-empty.
func HasEffectSections(s record.Snapshot) bool {
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

// maxVisibleProblems is the human default bound (OPEN-003). Structured snapshots
// retain full record.Problem lists separately (DEC-FAIL-001/003).
const maxVisibleProblems = 5

// Plain problem indent widths (fixed presentation dialect, not operational knobs).
const (
	// ProblemTreeIndent prefixes ├─ / └─ / │ problem rows.
	ProblemTreeIndent = "   "
	// problemDetailIndent continues multi-line Detail under a └─ / │ opener.
	problemDetailIndent = "      "
	// problemDetailGap separates a branch's continuation column from its
	// continuation text; the blank continuation of a closing branch plus
	// this gap is problemDetailIndent less the tree indent.
	problemDetailGap = "  "
	// TaskAnnotationIndent nests a standalone task's annotations — taxonomy
	// tallies, verification details, warnings, facts — under its row
	// (spec §26/§27: "✓ branches  50 checked" / "  ! kept 13 (...)").
	TaskAnnotationIndent = "  "
	// GroupChildIndent nests a Group header's children: its child rows and
	// the tallies its folded items leave behind, in one column.
	GroupChildIndent = "   "
)

// verifiedDetail is the muted suffix a satisfied attribute carries: the
// attribute was checked and holds, which says nothing about whether this run
// or an earlier one made it so. "already satisfied" is the Satisfied
// outcome's own wording and never appears here.
const verifiedDetail = "verified"

// verificationFactsGap separates a failed attribute's connector column from
// its Facts, so the Facts sit under the attribute's name.
const verificationFactsGap = "    "

// WriteVerificationDetails renders a Task's per-attribute reconciliation
// outcomes (spec §2, §8.2, §20-21) — evo.File/evo.Exec's third
// evidence layer, which subconditions were satisfied or failed, not just
// that the operation as a whole did. Each attribute is a branch of its
// Task's row, one level deeper than the row: a satisfied one reads
// "<name>  verified" in muted text; a failed one keeps its own glyph and
// nests whatever Facts explain it. Every attribute renders once the task
// itself failed — an isolated failure must never look like every attribute
// is suspect — and, mirroring Fact/Warning's own visibility rule, the same
// full list renders under Verbose even when the task succeeded; a clean run
// says nothing extra by default. prefix is the Task row's own prefix (the
// Group-child indent, or "" at the root), the column the row's Problems
// branch from too.
func WriteVerificationDetails(b *strings.Builder, details []record.VerificationDetail, prefix string, taskFailed bool, s Style) {
	writeVerificationBranches(b, VisibleVerificationDetails(details, taskFailed, s), prefix, s)
}

// VisibleVerificationDetails is the attributes a Task's row shows: all of
// them once the task failed or under Verbose, none otherwise.
func VisibleVerificationDetails(details []record.VerificationDetail, taskFailed bool, s Style) []record.VerificationDetail {
	if !taskFailed && !s.Verbose {
		return nil
	}
	return details
}

// writeVerificationBranches writes details as the last branches of a row's
// evidence tree: the final attribute closes it.
func writeVerificationBranches(b *strings.Builder, details []record.VerificationDetail, prefix string, s Style) {
	for i, d := range details {
		branch := newEvidenceBranch(i == len(details)-1, s)
		if d.Status == record.VerificationSatisfied {
			fmt.Fprintf(b, "%s%s%s %s\n", prefix, ProblemTreeIndent, branch.glyph, s.Dim(d.Name+"  "+verifiedDetail))
			continue
		}
		fmt.Fprintf(b, "%s%s%s %s %s\n", prefix, ProblemTreeIndent, branch.glyph, s.StateGlyph(record.Failed), d.Name)
		writeVerificationFacts(b, d.Facts, prefix+ProblemTreeIndent+branch.continuation+verificationFactsGap)
	}
}

// writeVerificationFacts renders one failed VerificationDetail's own Facts
// (error, path, mode, ...) beneath its attribute row, each name padded to
// the widest name in this detail alone so the values line up in one column
// (the same PadRight alignment convention taxonomy/effect columns already
// use).
func writeVerificationFacts(b *strings.Builder, facts []record.Fact, indent string) {
	width := 0
	for _, f := range facts {
		if w := Cells(f.Name); w > width {
			width = w
		}
	}
	for _, f := range facts {
		fmt.Fprintf(b, "%s%s  %s\n", indent, PadRight(f.Name, width), f.Value)
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
func WriteTask(b *strings.Builder, t record.TaskSnapshot, s Style) {
	WriteTaskAligned(b, t, 0, s)
}

// WriteTaskAligned renders a root task row with its name padded to
// nameWidth (0 = no padding) before any annotation, so a run of sibling
// tasks with inline warnings/facts line up in one column ("✓ branches
// ! kept 13...", fixture-repo-retire-dryrun.md). See taskRow.
func WriteTaskAligned(b *strings.Builder, t record.TaskSnapshot, nameWidth int, s Style) {
	RootRow(t, nameWidth).Write(b, s)
}

// runningTaskDetail composes a record.Running task's plain-mode detail text: its
// progress count (or Bytes fraction), its phase, or both together
// ("14/40  requests") — the durable-line counterpart of writeLiveTaskLine's
// interactive combination, without the live-only bar/heartbeat decoration.
// Returns "" for a record.Running task with neither (never happens through the
// public API, since every path that promotes record.Pending to record.Running sets one).
func runningTaskDetail(t record.TaskSnapshot) string {
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
// byte-fraction (BytesKind) count text a record.Running row shows, or "" when p
// carries neither — the one place that decides "does this progress have a
// displayable count," shared by runningTaskDetail (record.Running) and writeTask's
// record.Failed row (release-gate round 8 finding 4) so both projections agree on
// where and how the count reads.
func ProgressCountText(p record.Progress) string {
	switch {
	case p.Kind == record.BytesKind && p.Total > 0:
		return FormatByteProgressFixed(p.Completed, p.Total)
	case p.Kind == record.Determinate && p.Total > 0:
		return fmt.Sprintf("%d/%d", p.Completed, p.Total)
	default:
		return ""
	}
}

// maxVisibleEffectRows bounds how many plan/changes rows the human view
// renders per section (evo-rec.md "bound visible named rows... model
// keeps all records"). The snapshot always retains the full record list;
// only this presentation loop is capped. Mirrors maxVisibleProblems's bound.
const maxVisibleEffectRows = maxVisibleProblems
