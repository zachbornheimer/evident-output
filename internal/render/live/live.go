// Package live projects a core.Snapshot to interactive TTY frames — the
// counterpart to render/plain's durable text. It imports render for the
// shared row vocabulary; render never imports live (see render/plain's
// package doc, plain.go, for the mirrored rule on the plain side).
package live

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// elapsedAfter is evo-rec.md Problem 9's one monotonic elapsed-time
// mechanism: any unresolved row (Running or Pending), and any unfinished
// container header, gains an elapsed-time suffix ("pushing feat/a — 5s")
// once this long has passed since the row was first actually painted in the
// live region. It is a single honest clock, not a staleness heuristic —
// unlike the old phaseStaleAfter heartbeat, it never resets on Phase/Progress
// activity (see the root package's taskState.stampLiveFirstSeen, the only anchor
// this timer reads).
const elapsedAfter = 5 * time.Second

// activitySince anchors heartbeatSuffix's elapsed measurement to the row's
// first live-region render (taskState.stampLiveFirstSeen) — never to
// ActivityAt, so Phase/Progress calls (P5: "never resets on Phase/Progress
// activity") cannot restart the clock, and a core.Pending row, which never
// gets ActivityAt, still ages honestly from the moment it became visible.
func activitySince(t core.TaskSnapshot) time.Time {
	return t.LiveFirstSeenAt()
}

// heartbeatSuffix returns " — <elapsed>" once since has aged past
// elapsedAfter, or "" otherwise (including a zero since — a row not yet
// painted in the live region gets no heartbeat).
func heartbeatSuffix(now, since time.Time) string {
	if since.IsZero() {
		return ""
	}
	elapsed := now.Sub(since)
	if elapsed < elapsedAfter {
		return ""
	}
	return " — " + formatElapsed(elapsed)
}

// formatElapsed renders a compact, second-rounded duration: "45s" under a
// minute, "1m30s"/"2m3s" (Go's Duration.String shape) at or past a minute.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return d.String()
}

// renderLiveRegion builds the interactive ledger text for the current snapshot.
// now selects spinner frames (inject FixedClock in tests for stable glyphs).
// color applies SGR to glyphs as rows resolve (✓ green, ✗ red, spinner cyan).
func Region(s core.Snapshot, height, width int, now time.Time, style render.Style) string {
	height = liveHeight(height)
	var b strings.Builder
	style.Verbose = false
	st := liveStyle{Style: style, width: width, spin: txt.SpinnerGlyph(now, style.Profile), now: now}

	writeLiveBody(&b, liveRoot(render.QualifyFlattenedRows(s, liveHeaderRule(height))), height, atRoot, st)
	if render.HasTaskRows(s) && render.HasEffectSections(s) {
		b.WriteByte('\n')
	}
	render.WriteLedger(&b, s, width, style)
	return strings.TrimRight(b.String(), "\n")
}

// defaultLiveHeight is the frame height when the surface reports none.
const defaultLiveHeight = 24

// liveHeight is the frame height a live region of height rows paints.
func liveHeight(height int) int {
	if height <= 0 {
		return defaultLiveHeight
	}
	return height
}

// renderArmedTitleLine is the honest placeholder painted after arm() when the
// caller has not declared any entity yet — e.g. still parsing config. Falls
// back to a generic label rather than an empty string so the paint stays
// honest (never blank) even before Config.Title is known.
func ArmedTitleLine(subject string, now time.Time, s render.Style) string {
	title := subject
	if title == "" {
		title = "starting"
	}
	return fmt.Sprintf("%s  %s", txt.StyleGlyph(txt.SpinnerGlyph(now, s.Profile), txt.SGRCyan, s.Color), title)
}

func FitRegion(text string, columns int) string {
	if columns <= 0 {
		columns = render.DefaultWidth
	}
	if liveRegionFitsColumns(text, columns) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = txt.TruncateVisible(line, columns)
	}
	return strings.Join(lines, "\n")
}

// liveRegionFitsColumns reports whether every line of text already fits
// within columns, without allocating a Split slice.
func liveRegionFitsColumns(text string, columns int) bool {
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' {
			continue
		}
		if txt.VisibleCells(text[start:i]) > columns {
			return false
		}
		start = i + 1
	}
	return txt.VisibleCells(text[start:]) <= columns
}

// liveGroupHeader is a live Group's header row. While any child is
// unresolved it spins and carries "N/M complete" plus the one monotonic
// elapsed clock (P5), anchored to the earliest live-first-seen time among
// its (recursive) children — when this header itself first painted. Once
// every child has settled (spec §18's worked example: "✓ launch agent",
// no count) the count is redundant with the glyph and disappears.
func liveGroupHeader(col core.TasksSnapshot, done, total int, st liveStyle) render.DisplayUnit {
	spin, color, now, profile := st.spin, st.Color, st.now, st.Profile
	glyph, state := render.TaskGlyph(col.State, profile), col.State
	if col.State == core.Failed {
		glyph = txt.GlyphFailedState.Render(profile)
	}
	unresolved := anyChildRunning(col) || anyChildPendingActive(col)
	if unresolved {
		glyph, state = spin, core.Running
	}
	unit := render.DisplayUnit{Glyph: txt.StyleGlyph(glyph, render.StateColor(state), color), Name: col.Name}
	if unresolved {
		unit.Elapsed = heartbeatSuffix(now, earliestLiveFirstSeen(col))
		unit.Detail = fmt.Sprintf("%d/%d complete", done, total) + unit.Elapsed
	}
	return unit
}

// categoryStillClassifying reports whether col's own state is still in
// flight — any child Running or Pending, recursively. Contract §18: a
// folded "- skipped N" / "! kept N" tally names a final count, so it must
// not paint while more render.Disposition items could still arrive from work the
// category itself has not finished (the same reasoning the own-Task and
// promoted-lone-child live shapes already apply to their own row).
func categoryStillClassifying(col core.TasksSnapshot) bool {
	return anyChildRunning(col) || anyChildPendingActive(col)
}

func anyChildRunning(col core.TasksSnapshot) bool {
	if ownCounts(col).Running || core.CollectionTallyOf(col).Tasks.Running {
		return true
	}
	return slices.ContainsFunc(col.Collections, anyChildRunning)
}

// anyChildPendingActive reports whether the collection has any unresolved
// child — core.Running, or core.Pending regardless of Phase. A core.Pending child that
// never called Phase is the ordinary case (Phase/core.Progress are the only
// core.Running promoters), so requiring Phase here used to leave an all-core.Pending
// or core.Pending-tailed collection's header frozen on derivedState's static
// core.Incomplete glyph (evo-rec.md core.Problem 9). Recurses into nested
// containers (P3) so a still-pending grandchild keeps the root header honest.
func anyChildPendingActive(col core.TasksSnapshot) bool {
	if counts := ownCounts(col); counts.Running || counts.Pending {
		return true
	}
	if left := core.CollectionTallyOf(col).Tasks; left.Running || left.Pending {
		return true
	}
	return slices.ContainsFunc(col.Collections, anyChildPendingActive)
}

// earliestLiveFirstSeen returns the earliest LiveFirstSeenAt among a
// container's tasks and (recursively) its nested containers — the moment
// this header itself was first actually painted, and so the anchor its own
// elapsed-time suffix measures from (P5).
func earliestLiveFirstSeen(col core.TasksSnapshot) time.Time {
	earliest := earlierSeen(ownCounts(col).EarliestSeen, core.CollectionTallyOf(col).Tasks.EarliestSeen)
	for _, child := range col.Collections {
		earliest = earlierSeen(earliest, earliestLiveFirstSeen(child))
	}
	return earliest
}

// earlierSeen is the earlier of two live-first-seen stamps, where zero
// means never seen.
func earlierSeen(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// attentionRankCount is how many liveRank classes can fill a frame's rows
// once a collection has more children than fit: failed, warned, running,
// pending.
const attentionRankCount = 4

// liveRank orders a child for the rows of a frame too small for all of
// them: failed, warning, active (running), pending, successful, other.
// Warning is a Done-task annotation (P2), not a lifecycle state, so it
// ranks ahead of state on len(t.Warnings) rather than t.State.
func liveRank(t core.TaskSnapshot) int {
	switch {
	case t.State == core.Failed:
		return 0
	case len(t.Warnings) > 0:
		return 1
	case t.State == core.Running:
		return 2
	case t.State == core.Pending:
		return 3
	case t.State == core.Done, t.State == core.Skipped:
		return 4
	default:
		return 5
	}
}

// selectLiveChildren picks at most max of a collection's total children
// to show, from tasks, which holds all of them or the ones a Children
// projection kept.
func selectLiveChildren(tasks []core.TaskSnapshot, total, max int) (selected []core.TaskSnapshot, omitted int) {
	if total <= max {
		return tasks, 0
	}
	// Stable: collect by rank preserving declaration order within class.
	// Only the attention ranks can be selected (below), so routine rows —
	// nearly every row of a large finished Group — are never copied.
	var buckets [attentionRankCount][]core.TaskSnapshot
	for _, t := range tasks {
		if r := liveRank(t); r < attentionRankCount {
			buckets[r] = append(buckets[r], t)
		}
	}
	// Once there are more children than fit (the len(tasks) <= max early
	// return above did not apply), only the attention ranks (0-3: failed,
	// warning, running, pending) ever fill the budget — never rank 4/5
	// (routine Done/Skipped/other). A large aggregated Group answers "what
	// needs my attention" plus the header's own N/total count; padding the
	// remaining rows with routine "✓ package-NNN" landmarks merely because
	// there happens to be vertical room left repeats what the header already
	// said, one row at a time, for a Group large enough that its own
	// aggregation was already necessary (spec §25: "aggregation is
	// renderer-owned and automatic").
	for r := 0; r < attentionRankCount && len(selected) < max; r++ {
		for _, t := range buckets[r] {
			if len(selected) >= max {
				break
			}
			selected = append(selected, t)
		}
	}
	return selected, total - len(selected)
}

// writeLiveTaskLine renders one task row at the given indent and reports
// how many rows it wrote: the row, plus any activity child, nested
// warnings, and verification detail rows beneath it.
//
// A Running task with a determinate bar/count AND a current-activity Phase
// gets spec §23's stable-parent-plus-one-activity-child shape at any
// indent ("⠋ install dependencies  [████        ]  14/40  — 7s" /
// "  ⠋ urllib3" at root — contract §18's own frame — and the same pair
// indented three spaces further, six total, one level deeper under a
// Group):
// the parent line owns the bar/count/timer only, and the current activity
// becomes its own indented spinner line beneath it — so the child can
// change/truncate independently without moving the timer horizontally.
func writeLiveTaskLine(b *strings.Builder, t core.TaskSnapshot, indent, nameWidth int, cw countWidths, st liveStyle) (rows int) {
	start := b.Len()
	pad := ""
	// activityChildIndent is the extra indent the activity child adds on
	// top of pad. A root standalone row (indent == 0) has no pad of its
	// own, so its activity child's whole indent is this value — and
	// contract §18's normative frame pins that at two spaces
	// ("⠋ branches ...\n  ⠋ feat/style-contract"), not the three spaces a
	// nested Group child's own extra indent still uses below.
	activityChildIndent := "  "
	if indent > 0 {
		pad = "   "
		activityChildIndent = "   "
	}
	if splitsActivityChild(t) {
		parent := t
		parent.Phase = ""
		unit := liveTaskUnit(parent, indent, cw, st)
		padRootName(&unit, indent, nameWidth)
		b.WriteString(unit.Render(pad))
		b.WriteByte('\n')
		child := render.DisplayUnit{
			Glyph: txt.StyleGlyph(st.spin, render.StateColor(core.Running), st.Color),
			Name:  t.Phase,
		}
		b.WriteString(child.Render(pad + activityChildIndent))
		b.WriteByte('\n')
	} else {
		unit := liveTaskUnit(t, indent, cw, st)
		padRootName(&unit, indent, nameWidth)
		b.WriteString(unit.Render(pad))
		b.WriteByte('\n')
	}
	// Spec §22: warnings must not disappear. Running/Failed rows keep their
	// diagnostic parent line (bar/count or failure summary) and nest each
	// warning underneath — Done still inlines a short warning on the ✓ row.
	if t.State == core.Running || t.State == core.Failed {
		render.WriteNestedTaskWarnings(b, t.Warnings, pad+"   ", st.Style)
		// A standalone task's own accumulated Skipped/Kept taxonomy
		// (TaskHandle.Skipped/SkippedWithErrs called directly on this
		// task, not a Group folding still-arriving sibling children) is
		// already-final self-reported information the moment it is
		// recorded — the same footing as a Fact or a warning — so it
		// nests under the row unconditionally, unlike a Group's own
		// folded tally (categoryStillClassifying), which withholds
		// while more render.Disposition items could still arrive.
		render.WriteDispositions(b, pad+"   ", render.TaskDispositions(t), render.NoDisposition, st.Style)
	}
	if t.State == core.Failed {
		render.WriteVerificationDetails(b, t.Verification, pad+"   ", true, st.Style)
	}
	return rowsSince(b, start)
}

// padRootName right-pads a standalone (indent == 0) row's name to nameWidth
// (maxRootTaskNameWidth) — a no-op for a nested child row, which already
// has its own fixed-width padding from liveTaskUnit, or when there is no
// shared column to align (nameWidth == 0, a single standalone task).
func padRootName(unit *render.DisplayUnit, indent, nameWidth int) {
	if indent != 0 || nameWidth == 0 {
		return
	}
	unit.Name = txt.PadRight(unit.Name, nameWidth)
}

// splitsActivityChild reports whether t is a Running task with a
// determinate progress bar/count AND a current-activity Phase — the
// condition writeLiveTaskLine splits into a parent bar/count/timer line
// plus its own activity-child line, at any indent (spec §23).
func splitsActivityChild(t core.TaskSnapshot) bool {
	return t.State == core.Running &&
		t.Progress.Kind == core.Determinate &&
		t.Progress.Total > 0 &&
		t.Phase != ""
}

// liveTaskUnit composes one task row's render.DisplayUnit (P3's uniform row
// model): the glyph and name slots here, the detail slot by state below.
// Every case is a slot-filling policy — which fields get populated for
// this state/progress/indent combination — not a bespoke format string;
// render.DisplayUnit.Render owns the one line grammar. Returning the unit rather
// than writing it lets a caller that owns a richer row (a group header
// promoting its only Running child) reuse the whole policy and re-label
// just the name slot.
func liveTaskUnit(t core.TaskSnapshot, indent int, cw countWidths, st liveStyle) render.DisplayUnit {
	glyph := render.TaskGlyph(t.State, st.Profile)
	if t.State == core.Running {
		glyph = st.spin
	}
	unit := render.DisplayUnit{Glyph: txt.StyleGlyph(glyph, render.StateColor(t.State), st.Color), Name: t.Name}
	// Child rows pad their name to 9 for stable columns ("react" and
	// "sharp" share alignment; "esbuild" fills the field); a standalone
	// row keeps the bare name.
	if indent > 0 {
		unit.Name = txt.PadRight(t.Name, 9)
	}
	switch t.State {
	case core.Running:
		unit.Detail, unit.Elapsed = liveRunningDetail(t, cw, st)
	case core.Pending:
		unit.Detail = livePendingDetail(t, st)
	case core.Failed:
		unit.Detail = liveFailedDetail(t)
	default:
		unit.Detail = liveSettledDetail(t, st)
	}
	return unit
}

// liveSettledDetail is a finished row's detail: its byte total, the
// already-satisfied resolution, its Summary, or its warnings — one line
// per row live, so more than one warning names the first and counts the
// rest (plain mode nests them instead).
func liveSettledDetail(t core.TaskSnapshot, st liveStyle) string {
	switch {
	case t.State == core.Done && t.Progress.Kind == core.BytesKind:
		return formatBytes(t.Progress.Completed)
	case t.Resolution == core.ResolutionAlreadySatisfied:
		return render.AlreadySatisfiedRowDetail(t, st.Color)
	case t.State == core.Done && t.Summary != "":
		return st.Dim(t.Summary)
	case t.State == core.Done && len(t.Warnings) > 0:
		msg := render.WarningText(t.Warnings[0])
		if more := len(t.Warnings) - 1; more > 0 {
			msg = fmt.Sprintf("%s (+%d more)", msg, more)
		}
		return st.Dim(msg)
	default:
		return ""
	}
}

// liveRunningDetail is a Running row's bar/count or phase, and the elapsed
// suffix every unresolved row earns past elapsedAfter (P5), with or
// without a Phase.
func liveRunningDetail(t core.TaskSnapshot, cw countWidths, st liveStyle) (detail, elapsed string) {
	elapsed = heartbeatSuffix(st.now, activitySince(t))
	p := t.Progress
	switch {
	case p.Kind == core.BytesKind && p.Total > 0:
		return progressBar(p.Completed, p.Total, 12) + "  " + render.FormatByteProgressFixed(p.Completed, p.Total) + elapsed, elapsed
	case p.Kind == core.Determinate && p.Total > 0:
		detail := liveCountDetail(t, cw, st)
		// The count field's fixed one-space gap belongs to this
		// composition, not to formatAlignedCount's own field: it exists
		// only to combine with heartbeatSuffix's leading space into
		// spec §18's two-space gap before "— Ns" ("14/40  — 7s"), and
		// only when nothing else (a Phase) already sits between the
		// count and the elapsed suffix. This applies to every determinate
		// row alike, aligned or lone — §18's frame draws no distinction
		// by sibling count, so the gap must not either (a prior form
		// keyed this off cw.done, which both read the wrong width once
		// liveCountDetail zeroed cw for a narrow terminal, and left lone
		// rows one space short of aligned ones for no documented reason).
		// Before elapsedAfter, elapsed is still "" and there is nothing to
		// combine with, so a row never ends its line on bare trailing
		// whitespace for the run's first few seconds.
		if elapsed != "" {
			if t.Phase == "" {
				detail += " "
			}
			detail += elapsed
		}
		return detail, elapsed
	case p.Kind == core.BytesKind && t.Phase == "":
		// A byte stream with no known total and no phase has nothing to
		// show yet.
		return "", ""
	default:
		// Indeterminate, or Determinate with nothing to divide by
		// (Progress(0,0)): no count or bar, only the phase and heartbeat.
		phase := t.Phase
		if phase == "" {
			phase = "working…"
		}
		return st.Dim(phase) + elapsed, elapsed
	}
}

// liveCountDetail is a determinate Running row's "[bar]  N/M" and its
// muted current Phase. Narrow terminals drop the bar (decoration) before
// the count (information): evo-rec.md Problem 16/26's compact dialect.
func liveCountDetail(t core.TaskSnapshot, cw countWidths, st liveStyle) string {
	wide := st.width <= 0 || st.width >= render.CompactLayoutMaxWidth
	if !wide {
		// Narrow terminals drop the bar (decoration) before the count
		// (information): evo-rec.md Problem 16/26's compact dialect. The
		// shared count column is a full-width bar-row alignment (§18); a
		// compact row keeps its own bare "N/M".
		cw = countWidths{}
	}
	detail := formatAlignedCount(t.Progress.Completed, t.Progress.Total, cw)
	if wide {
		detail = progressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " + detail
	}
	if t.Phase != "" {
		detail += "  " + st.Dim(t.Phase)
	}
	return detail
}

// countWidths is the shared count-column width of a group of sibling
// determinate Running rows (§18's "count field" alignment): the widest
// completed digit count (right-justified) and the widest total digit count
// (left-justified), computed once across the siblings that share a name
// column. Zero when fewer than two rows share it — a lone determinate row
// keeps its own bare "N/M" width, unpadded.
type countWidths struct {
	done, total int
}

// formatAlignedCount is a determinate Running row's count field: doneStr
// right-justified to cw.done + "/" + totalStr left-justified to cw.total.
// It carries no trailing space of its own — liveRunningDetail's composition
// owns the fixed gap that combines with heartbeatSuffix's leading space into
// §18's two-space gap before "— Ns" once an elapsed suffix exists to combine
// with. With no shared column (cw.done == 0) it degrades to the bare "N/M"
// every other row already used, with no padding.
func formatAlignedCount(completed, total int64, cw countWidths) string {
	doneStr, totalStr := fmt.Sprintf("%d", completed), fmt.Sprintf("%d", total)
	if cw.done == 0 {
		return doneStr + "/" + totalStr
	}
	return txt.PadLeft(doneStr, cw.done) + "/" + txt.PadRight(totalStr, cw.total)
}

// headerlessCountWidths is the shared count-column width (countWidths) of a
// header-less body's determinate Running rows: its own Tasks plus every
// child collection that renders as its own Task row (render.RendersAsOwnTask).
// Fewer than two such rows share no column, matching
// render.HeaderlessRowNameWidth's own convention.
func headerlessCountWidths(col core.TasksSnapshot) countWidths {
	var doneWidth, totalWidth, rows int
	consider := func(t core.TaskSnapshot) {
		if t.State != core.Running || t.Progress.Kind != core.Determinate || t.Progress.Total <= 0 {
			return
		}
		rows++
		doneWidth = max(doneWidth, len(fmt.Sprintf("%d", t.Progress.Completed)))
		totalWidth = max(totalWidth, len(fmt.Sprintf("%d", t.Progress.Total)))
	}
	for _, t := range col.Tasks {
		consider(t)
	}
	for _, child := range col.Collections {
		stripped, _ := render.WithoutDispositionItems(child)
		if render.RendersAsOwnTask(stripped) {
			consider(stripped.Tasks[0])
		}
	}
	if rows < 2 {
		return countWidths{}
	}
	return countWidths{done: doneWidth, total: totalWidth}
}

// livePendingDetail says "waiting", dim, once a Pending row has stayed on
// screen past elapsedAfter. It carries no elapsed suffix: a queued row
// accumulates no work time, and three "waiting — 12s" siblings read as
// three stalled jobs rather than one queue.
func livePendingDetail(t core.TaskSnapshot, st liveStyle) string {
	if heartbeatSuffix(st.now, activitySince(t)) == "" {
		return ""
	}
	return st.Dim("waiting")
}

// liveFailedDetail is a Failed row's headline, after the count it reached
// when it failed mid-loop (release-gate round 8 finding 4).
func liveFailedDetail(t core.TaskSnapshot) string {
	msg := render.Headline(t)
	count := render.ProgressCountText(t.Progress)
	switch {
	case msg != "" && count != "":
		return count + "  " + msg
	case msg != "":
		return msg
	default:
		return count
	}
}

// progressBar returns a fixed-width ASCII bar for completed/total.
func progressBar(completed, total int64, width int) string {
	if width < 4 {
		width = 4
	}
	if total <= 0 {
		return "[" + strings.Repeat("?", width) + "]"
	}
	// §18's frame is normative: 120/459 -> 4/12 filled, 70/294 -> 3/12
	// filled, 1/4 -> 3/12 filled (exact). Any nonzero fraction of a cell
	// counts as that cell started — ceiling of completed/total*width — so
	// the bar never under-represents real progress the way nearest-value
	// rounding would (nearest would round 120/459 down to 3/12, hiding
	// work that has, in fact, started on a 4th cell).
	filled := int(math.Ceil(float64(width) * float64(completed) / float64(total)))
	if completed > 0 && filled == 0 {
		filled = 1
	}
	if completed < total && filled >= width {
		filled = width - 1
	}
	if filled > width {
		filled = width
	}
	// Empty cells are literal spaces (spec §23: "never shaded/outline
	// glyphs") — a shaded "░" cell reads as its own semantic state (some
	// libraries use it for "paused"/"buffered"), which the bar does not
	// have and must not imply.
	return "[" + strings.Repeat("█", filled) + strings.Repeat(" ", width-filled) + "]"
}

func formatBytes(n int64) string {
	const mb = 1000 * 1000
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	const kb = 1000
	if n >= kb {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
	return fmt.Sprintf("%d B", n)
}
