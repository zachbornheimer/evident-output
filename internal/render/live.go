package render

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
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

// quietAfter is how long a Writer-backed Running row goes without a new
// completed line before its owner row says so (ZYS-1045, v9b "Quiet").
const quietAfter = 60 * time.Second

// quietSuffix is " · quiet <duration>" for a Running row whose Writer has
// produced lines but none for quietAfter, in the warn color; "" for every
// other row. A row that never received a Writer line carries a zero
// LastLineAt and is never quiet.
func quietSuffix(t core.TaskSnapshot, st liveStyle) string {
	last := runningTail(t).LastLineAt
	if last.IsZero() {
		return ""
	}
	silent := st.now.Sub(last)
	if silent < quietAfter {
		return ""
	}
	return " " + txt.Style("· quiet "+formatQuiet(silent), txt.SGRYellow, st.Color)
}

// withQuietSuffix appends the quiet suffix to a Running row's detail.
func withQuietSuffix(detail string, t core.TaskSnapshot, st liveStyle) string {
	quiet := quietSuffix(t, st)
	if detail == "" {
		return strings.TrimPrefix(quiet, " ")
	}
	return detail + quiet
}

// formatQuiet renders a silence in whole minutes, or hours and minutes past
// an hour: "1m", "6m", "2h10m", "2h".
func formatQuiet(d time.Duration) string {
	minutes := int(d / time.Minute)
	hours, minutes := minutes/60, minutes%60
	switch {
	case hours == 0:
		return fmt.Sprintf("%dm", minutes)
	case minutes == 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
}

// renderLiveRegion builds the interactive ledger text for the current snapshot.
// now selects spinner frames (inject FixedClock in tests for stable glyphs).
// color applies SGR to glyphs as rows resolve (✓ green, ✗ red, spinner cyan).
func LiveRegion(s core.Snapshot, height, width int, now time.Time, style Style) string {
	height = liveHeight(height)
	var b strings.Builder
	style.Verbose = false
	st := liveStyle{Style: style, width: width, spin: txt.SpinnerGlyph(now, style.Profile), now: now}

	writeLiveBody(&b, liveRoot(QualifyFlattenedRows(s, liveHeaderRule(height))), height, atRoot, st)
	if HasTaskRows(s) && HasEffectSections(s) {
		b.WriteByte('\n')
	}
	WriteLedger(&b, s, width, style)
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
func ArmedTitleLine(subject string, now time.Time, s Style) string {
	title := subject
	if title == "" {
		title = "starting"
	}
	return fmt.Sprintf("%s  %s", txt.StyleGlyph(txt.SpinnerGlyph(now, s.Profile), txt.SGRCyan, s.Color), title)
}

func FitLiveRegion(text string, columns int) string {
	if columns <= 0 {
		columns = DefaultWidth
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
func liveGroupHeader(col core.TasksSnapshot, done, total int, st liveStyle) DisplayUnit {
	spin, color, now, profile := st.spin, st.Color, st.now, st.Profile
	glyph, state := TaskGlyph(col.State, profile), col.State
	if col.State == core.Failed {
		glyph = txt.GlyphFailedState.Render(profile)
	}
	unresolved := anyChildRunning(col) || anyChildPendingActive(col)
	if unresolved {
		glyph, state = spin, core.Running
	}
	unit := DisplayUnit{Glyph: txt.StyleGlyph(glyph, StateColor(state), color), Name: col.Name}
	if unresolved {
		unit.Elapsed = heartbeatSuffix(now, earliestLiveFirstSeen(col))
		unit.Detail = fmt.Sprintf("%d/%d complete", done, total) + unit.Elapsed
	}
	return unit
}

func anyChildRunning(col core.TasksSnapshot) bool {
	if OwnCounts(col).Running || core.CollectionTallyOf(col).Tasks.Running {
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
	if counts := OwnCounts(col); counts.Running || counts.Pending {
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
	earliest := earlierSeen(OwnCounts(col).EarliestSeen, core.CollectionTallyOf(col).Tasks.EarliestSeen)
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
// to show, from tasks, which holds all of them or the ones a LiveChildren
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
// "  ⠋ urllib3", and the same pair one level deeper under a Group):
// the parent line owns the bar/count/timer only, and the current activity
// becomes its own indented spinner line beneath it — so the child can
// change/truncate independently without moving the timer horizontally.
func writeLiveTaskLine(b *strings.Builder, t core.TaskSnapshot, indent, nameWidth int, st liveStyle) (rows int) {
	start := b.Len()
	pad := ""
	if indent > 0 {
		pad = "   "
	}
	tail := runningTail(t)
	if len(tail.Lines) > 0 && t.Phase == tail.Lines[len(tail.Lines)-1] {
		// The newest tail line is already on screen beneath the row; the
		// owner row keeps its name, bar/count and timer only.
		t.Phase = ""
	}
	if splitsActivityChild(t) {
		parent := t
		parent.Phase = ""
		unit := liveTaskUnit(parent, indent, st)
		padRootName(&unit, indent, nameWidth)
		b.WriteString(renderOwnerRow(unit, pad, t, st))
		b.WriteByte('\n')
		child := DisplayUnit{
			Glyph: txt.StyleGlyph(st.spin, StateColor(core.Running), st.Color),
			Name:  t.Phase,
		}
		b.WriteString(child.Render(pad + "   "))
		b.WriteByte('\n')
	} else {
		unit := liveTaskUnit(t, indent, st)
		padRootName(&unit, indent, nameWidth)
		b.WriteString(renderOwnerRow(unit, pad, t, st))
		b.WriteByte('\n')
	}
	writeLiveTail(b, tail, pad+"   ", st)
	// Spec §22: warnings must not disappear. Running/Failed rows keep their
	// diagnostic parent line (bar/count or failure summary) and nest each
	// warning underneath — Done still inlines a short warning on the ✓ row.
	if t.State == core.Running || t.State == core.Failed {
		WriteNestedTaskWarnings(b, t.Warnings, pad+"   ", st.Style)
	}
	if t.State == core.Failed {
		WriteVerificationDetails(b, t.Verification, pad+"   ", true, st.Style)
	}
	return rowsSince(b, start)
}

// renderOwnerRow is unit's line. A quiet row is cut to the frame width from
// the right, so the suffix is what gives way and the glyph and name stay.
func renderOwnerRow(unit DisplayUnit, pad string, t core.TaskSnapshot, st liveStyle) string {
	row := unit.Render(pad)
	if quietSuffix(t, st) == "" {
		return row
	}
	return fitTailLine(row, 0, st.width)
}

// runningTail is the live tail a Running row draws beneath itself; a row
// in any other state shows none.
func runningTail(t core.TaskSnapshot) core.LiveTail {
	if t.State != core.Running {
		return core.LiveTail{}
	}
	return core.LiveTailOf(t)
}

// writeLiveTail writes tail's lines beneath their owner row, dim and cut to
// the frame width so a long child line never wraps, then one footer line
// with the Task's total evidence count when the tail shows only part of it.
func writeLiveTail(b *strings.Builder, tail core.LiveTail, indent string, st liveStyle) {
	if len(tail.Lines) == 0 {
		return
	}
	for _, line := range tail.Lines {
		b.WriteString(indent + st.Dim(fitTailLine(line, len(indent), st.width)) + "\n")
	}
	if tail.Evidence > len(tail.Lines) {
		footer := indent + txt.GlyphOverflow.Render(st.Profile) + " " + evidenceLinesText(tail.Evidence)
		b.WriteString(st.Dim(fitTailLine(footer, 0, st.width)) + "\n")
	}
}

// fitTailLine cuts line to what is left of width after indent cells.
func fitTailLine(line string, indent, width int) string {
	if width <= 0 {
		width = DefaultWidth
	}
	return txt.TruncateVisible(line, max(width-indent, 1))
}

// evidenceLinesText is the tail footer's count of lines in the Task's
// evidence. No EVO_VERBOSE hint: verbose only reveals Facts, not Capture.
func evidenceLinesText(n int) string {
	if n == 1 {
		return "1 line in evidence"
	}
	return fmt.Sprintf("%d lines in evidence", n)
}

// padRootName right-pads a standalone (indent == 0) row's name to nameWidth
// (maxRootTaskNameWidth) — a no-op for a nested child row, which already
// has its own fixed-width padding from liveTaskUnit, or when there is no
// shared column to align (nameWidth == 0, a single standalone task).
func padRootName(unit *DisplayUnit, indent, nameWidth int) {
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

// liveTaskUnit composes one task row's DisplayUnit (P3's uniform row
// model): the glyph and name slots here, the detail slot by state below.
// Every case is a slot-filling policy — which fields get populated for
// this state/progress/indent combination — not a bespoke format string;
// DisplayUnit.Render owns the one line grammar. Returning the unit rather
// than writing it lets a caller that owns a richer row (a group header
// promoting its only Running child) reuse the whole policy and re-label
// just the name slot.
func liveTaskUnit(t core.TaskSnapshot, indent int, st liveStyle) DisplayUnit {
	glyph := TaskGlyph(t.State, st.Profile)
	if t.State == core.Running {
		glyph = st.spin
	}
	unit := DisplayUnit{Glyph: txt.StyleGlyph(glyph, StateColor(t.State), st.Color), Name: t.Name}
	// Child rows pad their name to 9 for stable columns ("react" and
	// "sharp" share alignment; "esbuild" fills the field); a standalone
	// row keeps the bare name.
	if indent > 0 {
		unit.Name = txt.PadRight(t.Name, 9)
	}
	switch t.State {
	case core.Running:
		unit.Detail, unit.Elapsed = liveRunningDetail(t, st)
		unit.Detail = withQuietSuffix(unit.Detail, t, st)
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
		return FormatBytes(t.Progress.Completed)
	case t.Resolution == core.ResolutionAlreadySatisfied:
		return AlreadySatisfiedRowDetail(t, st.Color)
	case t.State == core.Done && t.Summary != "":
		return st.Dim(t.Summary)
	case t.State == core.Done && len(t.Warnings) > 0:
		msg := WarningText(t.Warnings[0])
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
func liveRunningDetail(t core.TaskSnapshot, st liveStyle) (detail, elapsed string) {
	elapsed = heartbeatSuffix(st.now, activitySince(t))
	p := t.Progress
	switch {
	case p.Kind == core.BytesKind && p.Total > 0:
		return ProgressBar(p.Completed, p.Total, 12) + "  " + FormatByteProgressFixed(p.Completed, p.Total) + elapsed, elapsed
	case p.Kind == core.Determinate && p.Total > 0:
		return liveCountDetail(t, st) + elapsed, elapsed
	case p.Kind == core.BytesKind && t.Phase == "":
		// A byte stream with no known total and no phase has nothing to
		// show yet.
		return "", ""
	default:
		// Indeterminate, or Determinate with nothing to divide by
		// (Progress(0,0)): no count or bar, only the phase and heartbeat.
		// A row with a live tail beneath it is already visibly working.
		phase := t.Phase
		if phase == "" && len(core.LiveTailOf(t).Lines) > 0 {
			return elapsed, elapsed
		}
		if phase == "" {
			phase = "working…"
		}
		return st.Dim(phase) + elapsed, elapsed
	}
}

// liveCountDetail is a determinate Running row's "[bar]  N/M" and its
// muted current Phase. Narrow terminals drop the bar (decoration) before
// the count (information): evo-rec.md Problem 16/26's compact dialect.
func liveCountDetail(t core.TaskSnapshot, st liveStyle) string {
	detail := fmt.Sprintf("%d/%d", t.Progress.Completed, t.Progress.Total)
	if st.width <= 0 || st.width >= CompactLayoutMaxWidth {
		detail = ProgressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " + detail
	}
	if t.Phase != "" {
		detail += "  " + st.Dim(t.Phase)
	}
	return detail
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
	msg := Headline(t)
	count := ProgressCountText(t.Progress)
	switch {
	case msg != "" && count != "":
		return count + "  " + msg
	case msg != "":
		return msg
	default:
		return count
	}
}
