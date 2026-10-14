package live

import (
	"fmt"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
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
// activity") cannot restart the clock, and a record.Pending row, which never
// gets ActivityAt, still ages honestly from the moment it became visible.
func activitySince(t record.TaskSnapshot) time.Time {
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
	return " — " + project.FormatElapsed(elapsed)
}

// quietAfter is how long a Writer-backed Running row goes without a new
// completed line before its owner row says so (ZYS-1045, v9b "Quiet").
const quietAfter = 60 * time.Second

// quietSuffix is " · quiet <duration>" for a Running row whose Writer has
// produced lines but none for quietAfter, in the warn color; "" for every
// other row. A row that never received a Writer line carries a zero
// LastLineAt and is never quiet.
func quietSuffix(t record.TaskSnapshot, st liveStyle) string {
	last := runningTail(t).LastLineAt
	if last.IsZero() {
		return ""
	}
	silent := st.now.Sub(last)
	if silent < quietAfter {
		return ""
	}
	return " " + project.ApplySGR("· quiet "+project.FormatElapsed(silent), project.SGRYellow, st.Color)
}

// withQuietSuffix appends the quiet suffix to a Running row's detail.
func withQuietSuffix(detail string, t record.TaskSnapshot, st liveStyle) string {
	quiet := quietSuffix(t, st)
	if detail == "" {
		return strings.TrimPrefix(quiet, " ")
	}
	return detail + quiet
}

// renderLiveRegion builds the interactive ledger text for the current snapshot.
// now selects spinner frames (inject FixedClock in tests for stable glyphs).
// color applies SGR to glyphs as rows resolve (✓ green, ✗ red, spinner cyan).
func LiveRegion(s record.Snapshot, height, width int, now time.Time, style project.Style) string {
	height = liveHeight(height)
	var b strings.Builder
	style.Verbose = false
	st := liveStyle{Style: style, width: width, spin: project.SpinnerGlyph(now, style.Profile), now: now}

	writeLiveBody(&b, liveRoot(s), height, atRoot, st)
	if project.HasTaskRows(s) && project.HasEffectSections(s) {
		b.WriteByte('\n')
	}
	project.WriteLedger(&b, s, width, style)
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
func ArmedTitleLine(subject string, now time.Time, s project.Style) string {
	title := subject
	if title == "" {
		title = "starting"
	}
	return fmt.Sprintf("%s  %s", project.StyleGlyph(project.SpinnerGlyph(now, s.Profile), project.SGRCyan, s.Color), title)
}

func FitLiveRegion(text string, columns int) string {
	if columns <= 0 {
		columns = project.DefaultWidth
	}
	if liveRegionFitsColumns(text, columns) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = project.TruncateVisible(line, columns)
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
		if project.VisibleCells(text[start:i]) > columns {
			return false
		}
		start = i + 1
	}
	return project.VisibleCells(text[start:]) <= columns
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
func writeLiveTaskLine(b *strings.Builder, t record.TaskSnapshot, indent, nameWidth int, st liveStyle) (rows int) {
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
		child := project.DisplayUnit{
			Glyph: project.StyleGlyph(st.spin, project.StateColor(record.Running), st.Color),
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
	if t.State == record.Running || t.State == record.Failed {
		project.WriteNestedTaskWarnings(b, t.Warnings, pad+"   ", st.Style)
	}
	if t.State == record.Failed {
		project.WriteVerificationDetails(b, t.Verification, pad+"   ", true, st.Style)
	}
	return rowsSince(b, start)
}

// renderOwnerRow is unit's line. A quiet row is cut to the frame width from
// the right, so the suffix is what gives way and the glyph and name stay.
func renderOwnerRow(unit project.DisplayUnit, pad string, t record.TaskSnapshot, st liveStyle) string {
	row := unit.Render(pad)
	if quietSuffix(t, st) == "" {
		return row
	}
	return fitTailLine(row, 0, st.width)
}

// runningTail is the live tail a Running row draws beneath itself; a row
// in any other state shows none.
func runningTail(t record.TaskSnapshot) record.LiveTail {
	if t.State != record.Running {
		return record.LiveTail{}
	}
	return record.LiveTailOf(t)
}

// writeLiveTail writes tail's lines beneath their owner row, dim and cut to
// the frame width so a long child line never wraps, then one footer line
// with the Task's total evidence count when the tail shows only part of it.
func writeLiveTail(b *strings.Builder, tail record.LiveTail, indent string, st liveStyle) {
	if len(tail.Lines) == 0 {
		return
	}
	for _, line := range tail.Lines {
		b.WriteString(indent + st.Dim(fitTailLine(line, len(indent), st.width)) + "\n")
	}
	if tail.Evidence > len(tail.Lines) {
		footer := indent + project.GlyphOverflow.Render(st.Profile) + " " + evidenceLinesText(tail.Evidence)
		b.WriteString(st.Dim(fitTailLine(footer, 0, st.width)) + "\n")
	}
}

// fitTailLine cuts line to what is left of width after indent cells.
func fitTailLine(line string, indent, width int) string {
	if width <= 0 {
		width = project.DefaultWidth
	}
	return project.TruncateVisible(line, max(width-indent, 1))
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
func padRootName(unit *project.DisplayUnit, indent, nameWidth int) {
	if indent != 0 || nameWidth == 0 {
		return
	}
	unit.Name = project.PadRight(unit.Name, nameWidth)
}

// splitsActivityChild reports whether t is a Running task with a
// determinate progress bar/count AND a current-activity Phase — the
// condition writeLiveTaskLine splits into a parent bar/count/timer line
// plus its own activity-child line, at any indent (spec §23).
func splitsActivityChild(t record.TaskSnapshot) bool {
	return t.State == record.Running &&
		t.Progress.Kind == record.Determinate &&
		t.Progress.Total > 0 &&
		t.Phase != ""
}

// liveTaskUnit composes one task row's project.DisplayUnit (P3's uniform row
// model): the glyph and name slots here, the detail slot by state below.
// Every case is a slot-filling policy — which fields get populated for
// this state/progress/indent combination — not a bespoke format string;
// project.DisplayUnit.Render owns the one line grammar. Returning the unit rather
// than writing it lets a caller that owns a richer row (a group header
// promoting its only Running child) reuse the whole policy and re-label
// just the name slot.
func liveTaskUnit(t record.TaskSnapshot, indent int, st liveStyle) project.DisplayUnit {
	glyph := project.TaskGlyph(t.State, st.Profile)
	if t.State == record.Running {
		glyph = st.spin
	}
	unit := project.DisplayUnit{Glyph: project.StyleGlyph(glyph, project.StateColor(t.State), st.Color), Name: t.Name}
	// Child rows pad their name to 9 for stable columns ("react" and
	// "sharp" share alignment; "esbuild" fills the field); a standalone
	// row keeps the bare name.
	if indent > 0 {
		unit.Name = project.PadRight(t.Name, 9)
	}
	switch t.State {
	case record.Running:
		unit.Detail, unit.Elapsed = liveRunningDetail(t, st)
		unit.Detail = withQuietSuffix(unit.Detail, t, st)
	case record.Pending:
		unit.Detail = livePendingDetail(t, st)
	case record.Failed:
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
func liveSettledDetail(t record.TaskSnapshot, st liveStyle) string {
	switch {
	case t.State.Succeeded() && t.Progress.Kind == record.BytesKind:
		return project.FormatBytes(t.Progress.Completed)
	case t.State == record.Satisfied:
		return project.AlreadySatisfiedRowDetail(t, st.Color)
	case t.State.Succeeded() && t.Summary != "":
		return st.Dim(t.Summary)
	case t.State.Succeeded() && len(t.Warnings) > 0:
		msg := project.WarningText(t.Warnings[0])
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
func liveRunningDetail(t record.TaskSnapshot, st liveStyle) (detail, elapsed string) {
	elapsed = heartbeatSuffix(st.now, activitySince(t))
	p := t.Progress
	switch {
	case p.Kind == record.BytesKind && p.Total > 0:
		return project.ProgressBar(p.Completed, p.Total, 12) + "  " + project.FormatByteProgressFixed(p.Completed, p.Total) + elapsed, elapsed
	case p.Kind == record.Determinate && p.Total > 0:
		return liveCountDetail(t, st) + elapsed, elapsed
	case p.Kind == record.BytesKind && t.Phase == "":
		// A byte stream with no known total and no phase has nothing to
		// show yet.
		return "", ""
	default:
		// Indeterminate, or Determinate with nothing to divide by
		// (Progress(0,0)): no count or bar, only the phase and heartbeat.
		// A row with a live tail beneath it is already visibly working.
		phase := t.Phase
		if phase == "" && len(record.LiveTailOf(t).Lines) > 0 {
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
func liveCountDetail(t record.TaskSnapshot, st liveStyle) string {
	detail := fmt.Sprintf("%d/%d", t.Progress.Completed, t.Progress.Total)
	if st.width <= 0 || st.width >= project.CompactLayoutMaxWidth {
		detail = project.ProgressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " + detail
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
func livePendingDetail(t record.TaskSnapshot, st liveStyle) string {
	if heartbeatSuffix(st.now, activitySince(t)) == "" {
		return ""
	}
	return st.Dim("waiting")
}

// liveFailedDetail is a Failed row's headline, after the count it reached
// when it failed mid-loop (release-gate round 8 finding 4).
func liveFailedDetail(t record.TaskSnapshot) string {
	msg := project.Headline(t)
	count := project.ProgressCountText(t.Progress)
	switch {
	case msg != "" && count != "":
		return count + "  " + msg
	case msg != "":
		return msg
	default:
		return count
	}
}
