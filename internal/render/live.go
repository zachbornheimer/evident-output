package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// phaseStaleAfter is how long a Running task's phase (and progress) can go
// unchanged before the live projection appends an elapsed-time heartbeat
// suffix ("pushing feat/a — 90s") — the spinner is animation, not evidence,
// so a silent child must not read the same as a healthy one. Resets on every
// Phase/Progress call (see the root package's taskState.activityAt).
const phaseStaleAfter = 10 * time.Second

// activitySince picks the anchor heartbeatSuffix measures elapsed time from:
// ActivityAt when a Phase/core.Progress call has actually happened, otherwise the
// row's first live-region render (see stampLiveFirstSeenLocked) — so a
// core.Pending row, which never gets ActivityAt, still ages honestly from the
// moment it became visible rather than never aging at all.
func activitySince(t core.TaskSnapshot) time.Time {
	if !t.ActivityAt.IsZero() {
		return t.ActivityAt
	}
	return t.LiveFirstSeenAt()
}

// heartbeatSuffix returns " — <elapsed>" once since has gone stale past
// phaseStaleAfter, or "" otherwise (including a zero since — a task that has
// never had Phase/core.Progress set gets no heartbeat).
func heartbeatSuffix(now, since time.Time) string {
	if since.IsZero() {
		return ""
	}
	elapsed := now.Sub(since)
	if elapsed < phaseStaleAfter {
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
func LiveRegion(s core.Snapshot, height, width int, now time.Time, color bool, profile txt.GlyphProfile) string {
	if height <= 0 {
		height = 24
	}
	var b strings.Builder
	spin := txt.SpinnerGlyph(now, profile)

	// Prefer collections for multi-task progress display.
	for _, col := range s.Collections {
		writeLiveCollection(&b, col, height, width, spin, color, now, profile)
	}
	for _, t := range s.Tasks {
		writeLiveTaskLine(&b, t, 0, width, spin, color, now, profile)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderArmedTitleLine is the honest placeholder painted after arm() when the
// caller has not declared any entity yet — e.g. still parsing config. Falls
// back to a generic label rather than an empty string so the paint stays
// honest (never blank) even before Config.Title is known.
func ArmedTitleLine(subject string, now time.Time, color bool, profile txt.GlyphProfile) string {
	title := subject
	if title == "" {
		title = "starting"
	}
	return fmt.Sprintf("%s  %s", txt.StyleGlyph(txt.SpinnerGlyph(now, profile), txt.SGRCyan, color), title)
}

func FitLiveRegion(text string, columns int) string {
	if columns <= 0 {
		columns = defaultWidth
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = txt.TruncateVisible(line, columns)
	}
	return strings.Join(lines, "\n")
}

func writeLiveCollection(b *strings.Builder, col core.TasksSnapshot, height, width int, spin string, color bool, now time.Time, profile txt.GlyphProfile) {
	done, total := 0, len(col.Tasks)
	for _, t := range col.Tasks {
		if t.State == core.Done || t.State == core.Skipped {
			done++
		}
	}
	// Header
	glyph := TaskGlyph(col.State, profile)
	if col.State == core.Failed || anyChildFailed(col) {
		if col.State == core.Failed {
			glyph = txt.GlyphFailedState.Render(profile)
		}
	}
	// When any running, animate header spinner (H.20 uses FixedClock → stable ⠋).
	if anyChildRunning(col) || anyChildPendingActive(col) {
		glyph = spin
	}
	headerState := col.State
	if anyChildRunning(col) || anyChildPendingActive(col) {
		headerState = core.Running
	}
	fmt.Fprintf(b, "%s  %s  %d/%d complete\n",
		txt.StyleGlyph(glyph, StateColor(headerState), color), col.Name, done, total)

	// Select children by severity under height budget.
	// Budget: height includes header; leave room for omission line.
	maxChildRows := height - 2 // header + possible omission
	if maxChildRows < 1 {
		maxChildRows = 1
	}
	selected, omitted := selectLiveChildren(col.Tasks, maxChildRows)
	for _, t := range selected {
		writeLiveTaskLine(b, t, 1, width, spin, color, now, profile)
	}
	if omitted > 0 {
		fmt.Fprintf(b, "   %s  %d not shown\n", txt.Dim(txt.GlyphOverflow.Render(profile), color), omitted)
	}
}

func anyChildRunning(col core.TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == core.Running {
			return true
		}
	}
	return false
}

func anyChildFailed(col core.TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == core.Failed {
			return true
		}
	}
	return false
}

// anyChildPendingActive reports whether the collection has any unresolved
// child — core.Running, or core.Pending regardless of Phase. A core.Pending child that
// never called Phase is the ordinary case (Phase/core.Progress are the only
// core.Running promoters), so requiring Phase here used to leave an all-core.Pending
// or core.Pending-tailed collection's header frozen on derivedState's static
// core.Incomplete glyph (evo-rec.md core.Problem 9).
func anyChildPendingActive(col core.TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == core.Running || t.State == core.Pending {
			return true
		}
	}
	return false
}

func selectLiveChildren(tasks []core.TaskSnapshot, max int) (selected []core.TaskSnapshot, omitted int) {
	if len(tasks) <= max {
		return tasks, 0
	}
	// Priority: failed, warning, active(running), pending, successful.
	// Warning is a Done-task annotation now (P2), not a lifecycle state, so
	// it ranks ahead of state on len(t.Warnings) rather than t.State.
	rank := func(t core.TaskSnapshot) int {
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
	// Stable: collect by rank preserving declaration order within class.
	var buckets [6][]core.TaskSnapshot
	for _, t := range tasks {
		r := rank(t)
		buckets[r] = append(buckets[r], t)
	}
	for r := 0; r < 6 && len(selected) < max; r++ {
		for _, t := range buckets[r] {
			if len(selected) >= max {
				break
			}
			selected = append(selected, t)
		}
	}
	return selected, len(tasks) - len(selected)
}

func writeLiveTaskLine(b *strings.Builder, t core.TaskSnapshot, indent, width int, spin string, color bool, now time.Time, profile txt.GlyphProfile) {
	pad := ""
	if indent > 0 {
		pad = "   "
	}
	glyph := TaskGlyph(t.State, profile)
	if t.State == core.Running {
		glyph = spin
	}
	g := txt.StyleGlyph(glyph, StateColor(t.State), color)
	// Child rows: indent + glyph + two spaces + name padded to 9 + two spaces + detail.
	// Produces stable columns: "react" and "sharp" share alignment; "esbuild" fills the field.
	nameField := t.Name
	if indent > 0 {
		nameField = txt.PadRight(t.Name, 9)
	}
	switch {
	case t.State == core.Done && t.Progress.Kind == core.BytesKind:
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, formatBytes(t.Progress.Completed))
	case t.State == core.Done && t.Summary != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, txt.Dim(t.Summary, color))
	case t.State == core.Running && t.Progress.Kind == core.BytesKind && t.Progress.Total > 0:
		detail := formatByteProgressFixed(t.Progress.Completed, t.Progress.Total)
		detail = progressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " + detail
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, detail)
	case t.State == core.Running && t.Progress.Kind == core.Determinate && t.Progress.Total > 0:
		count := fmt.Sprintf("%d/%d", t.Progress.Completed, t.Progress.Total)
		// Narrow terminals degrade by dropping decoration (the bar) before
		// information (count, name) — evo-rec.md core.Problem 16/26's compact
		// dialect. Below compactLayoutMaxWidth the fixed 12-cell bar is
		// exactly the kind of leader-only decoration the whole-line
		// truncation in fitLiveRegion would otherwise eat into first.
		detail := count
		if width <= 0 || width >= compactLayoutMaxWidth {
			detail = progressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " + count
		}
		if t.Phase != "" {
			// Default intensity: the current phase is diagnostic evidence
			// while progress stalls, not a subordinate row (evo-rec.md
			// "Color and txt.Style demotions").
			detail = detail + "  " + t.Phase + heartbeatSuffix(now, activitySince(t))
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, detail)
	case t.State == core.Running && (t.Progress.Kind == core.Indeterminate || t.Phase != "" || (t.Progress.Kind == core.Determinate && t.Progress.Total <= 0)):
		// core.Indeterminate, or core.Determinate with nothing to divide by (Total<=0,
		// e.g. core.Progress(0,0)): spinner glyph + phase (or generic working) —
		// folded together because neither has a renderable count/bar, so both
		// need the same "is this actually still moving?" heartbeat.
		phase := t.Phase
		if phase == "" {
			phase = "working…"
		}
		phase += heartbeatSuffix(now, activitySince(t))
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, phase)
	case t.State == core.Running && t.Phase != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, t.Phase)
	case t.State == core.Pending:
		// A core.Pending row left on screen past phaseStaleAfter is exactly as
		// static as a stalled core.Running one — same heartbeat, txt.Dim (subordinate:
		// nothing is happening yet) rather than the diagnostic-intensity
		// phase text a core.Running row gets.
		if hb := heartbeatSuffix(now, activitySince(t)); hb != "" {
			fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, txt.Dim("waiting"+hb, color))
		} else {
			fmt.Fprintf(b, "%s%s  %s\n", pad, g, strings.TrimRight(nameField, " "))
		}
	case t.State == core.Failed:
		msg := t.Summary
		if msg == "" && len(t.Problems) > 0 {
			msg = t.Problems[0].Summary
		}
		// release-gate round 8 finding 4: a task that failed mid-loop still
		// carries the in-flight count it had when Fail was called — render
		// it in the same position a core.Running row shows it (right after the
		// name), so the failure row never loses "how far did it get".
		count := progressCountText(t.Progress)
		switch {
		case msg != "" && count != "":
			fmt.Fprintf(b, "%s%s  %s  %s  %s\n", pad, g, nameField, count, msg)
		case msg != "":
			fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, msg)
		case count != "":
			fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, count)
		default:
			fmt.Fprintf(b, "%s%s  %s\n", pad, g, strings.TrimRight(nameField, " "))
		}
	case t.State == core.Done && len(t.Warnings) > 0:
		// A short, single warning inlines on the ✓ row (P2); with more than
		// one, name the first and count the rest — live is one line per row,
		// unlike plain mode's nested "!" lines.
		msg := t.Warnings[0].Summary
		if more := len(t.Warnings) - 1; more > 0 {
			msg = fmt.Sprintf("%s (+%d more)", msg, more)
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, txt.Dim(msg, color))
	default:
		fmt.Fprintf(b, "%s%s  %s\n", pad, g, strings.TrimRight(nameField, " "))
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
	filled := int(float64(width) * float64(completed) / float64(total))
	if completed > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
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

func formatByteProgressFixed(completed, total int64) string {
	const mb = 1_000_000.0
	return fmt.Sprintf("%.1f/%.1f MB", float64(completed)/mb, float64(total)/mb)
}
