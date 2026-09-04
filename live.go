package evo

import (
	"fmt"
	"strings"
	"sync"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// spinnerPeriod is the wall-clock duration between spinner frame advances.
const spinnerPeriod = 80 * time.Millisecond

// phaseStaleAfter is how long a Running task's phase (and progress) can go
// unchanged before the live projection appends an elapsed-time heartbeat
// suffix ("pushing feat/a — 90s") — the spinner is animation, not evidence,
// so a silent child must not read the same as a healthy one. Resets on every
// Phase/Progress call (see taskState.activityAt).
const phaseStaleAfter = 10 * time.Second

// LiveSurface is an interactive terminal sink for live-region rendering.
// testkit.Screen implements this; production drivers can as well.
type LiveSurface interface {
	TerminalDriver
	Columns() int
	Rows() int
	IsInteractive() bool
	WriteLive(text string)
	ClearLive()
	WriteDurable(line string)
	WriteFinal(text string)
}

// asLive returns a LiveSurface when the configured terminal supports it.
func asLive(d TerminalDriver) LiveSurface {
	if d == nil {
		return nil
	}
	if ls, ok := d.(LiveSurface); ok {
		return ls
	}
	return nil
}

type liveEngine struct {
	surface       LiveSurface
	visible       bool
	lastRender    time.Time
	lastLiveText  string
	liveActive    bool
	pendingRedraw bool

	// activitySince is set when live activity first appears; VisibilityDelay
	// withholds the first paint until the domain clock advances past the delay
	// (force terminal outcomes bypass the delay).
	activitySince time.Time
	waitingDelay  bool

	// anim drives independent spinner ticks while any task is Running,
	// so indeterminate rows animate even when determinate bars are idle.
	// Also used to promote visibility once VisibilityDelay elapses.
	animMu      sync.Mutex
	animRunning bool
	animStop    chan struct{}

	// resizeArmed is true when SIGWINCH watch is registered on the surface.
	resizeArmed bool
}

func (o *Output) liveLocked() LiveSurface {
	if o.cfg.plain {
		return nil
	}
	return asLive(o.cfg.terminal)
}

// signalLiveLocked marks that interactive presentation may need a redraw.
// force=true bypasses frame-rate coalescing only (not VisibilityDelay).
// VisibilityDelay withholds the first live paint after activity starts so
// Task/Phase→fast Done does not flash a spinner. Instant Done still waits
// the delay; if delay has not elapsed, no live frames (H.2).
func (o *Output) signalLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
		o.startResizeWatchLocked(live)
	}
	now := o.cfg.clock.Now()
	if o.hasLiveActivityLocked() {
		if o.live.activitySince.IsZero() {
			o.live.activitySince = now
		}
		delay := o.cfg.visibilityDelay
		// delay <= 0 means immediate (tests use VisibilityDelay(0)).
		if delay <= 0 || now.Sub(o.live.activitySince) >= delay {
			o.live.visible = true
			o.live.waitingDelay = false
		} else {
			o.live.waitingDelay = true
			// Wall ticker re-checks delay; FixedClock tests re-enter after Advance.
			o.ensureSpinnerAnimatorLocked()
			return
		}
	}
	if !o.live.visible {
		return
	}
	if !force && !o.live.lastRender.IsZero() {
		minGap := time.Second / time.Duration(max(1, o.cfg.maxFrameRate))
		if now.Sub(o.live.lastRender) < minGap {
			o.live.pendingRedraw = true
			return
		}
	}
	o.renderLiveLocked(force)
	o.ensureSpinnerAnimatorLocked()
}

func (o *Output) hasLiveActivityLocked() bool {
	for _, t := range o.tasks {
		// Pending counts too: a declared task renders its named "○" row
		// immediately (evo-rec.md "predeclare Tasks; ... others named
		// idle") — VisibilityDelay withholds the *spinner flash* for
		// near-instant work, not the fact that a task now exists.
		if t.state == Running || t.state == Pending || t.phase != "" {
			return true
		}
		if t.progress.Kind == Determinate || t.progress.Kind == BytesKind {
			if t.state == Running || t.state == Done || t.state == Failed || t.state == Warning {
				return true
			}
		}
	}
	// Armed-but-empty: Init promised a paint before any entity exists. Once
	// the first Task/Tasks is declared, its own state drives activity.
	if o.armed && len(o.tasks) == 0 && len(o.collections) == 0 {
		return true
	}
	return false
}

// arm marks the live surface as ready to paint before any entity is declared,
// so Init honors the ≤100ms first-paint contract even when the caller does
// heavy work before the first Task. Idempotent.
func (o *Output) arm() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.armed {
		return
	}
	o.armed = true
	o.signalLiveLocked(false)
}

func (o *Output) renderLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || o.live == nil || !o.live.visible {
		return
	}
	// Refresh geometry before layout when the driver supports it (ANSI + real TTY).
	if r, ok := live.(interface{ RefreshSize() }); ok {
		r.RefreshSize()
	}
	now := o.cfg.clock.Now()
	cols, rows := live.Columns(), live.Rows()
	// Keep config width in sync for plain residual paths that still use cfg.text.
	if cols > 0 {
		o.cfg.width = cols
	}
	o.stampLiveFirstSeenLocked(now)
	text := o.renderLiveRegionWithDebugLocked(cols, rows, now)
	if !force && text == o.live.lastLiveText {
		return
	}
	live.WriteLive(text)
	o.live.lastLiveText = text
	o.live.lastRender = now
	o.live.liveActive = true
	o.live.pendingRedraw = false
}

// needsSpinnerAnimLocked reports whether any live row should keep ticking: a
// Pending or Running task that is still actually rendered in the current
// frame — not a standalone task already flushed to durable text and dropped
// from the ticker (see liveTickerSnapshotLocked's matching filter). Counting
// only Running here used to let an all-Pending frame (nothing has started
// yet, or every child is still waiting) freeze forever (evo-rec.md Problem
// 9's universal heartbeat).
func (o *Output) needsSpinnerAnimLocked() bool {
	for _, t := range o.tasks {
		if t.state != Running && t.state != Pending {
			continue
		}
		if t.collection == nil && t.coreEmitted {
			continue
		}
		return true
	}
	return false
}

// ensureSpinnerAnimatorLocked starts a background tick that re-renders the live
// region so indeterminate spinners advance without waiting for Progress calls,
// and that promotes visibility once VisibilityDelay elapses.
func (o *Output) ensureSpinnerAnimatorLocked() {
	if o.live == nil || o.finished || o.closed {
		return
	}
	// Waiting for delay: keep a ticker so we can paint when the threshold elapses.
	switch {
	case o.live.waitingDelay:
		// fall through to start animator
	case !o.live.visible:
		return
	case !o.needsSpinnerAnimLocked():
		o.stopSpinnerAnimatorLocked()
		return
	}
	o.live.animMu.Lock()
	if o.live.animRunning {
		o.live.animMu.Unlock()
		return
	}
	stop := make(chan struct{})
	o.live.animStop = stop
	o.live.animRunning = true
	o.live.animMu.Unlock()
	go o.spinnerAnimateLoop(stop)
}

func (o *Output) stopSpinnerAnimatorLocked() {
	if o.live == nil {
		return
	}
	o.live.animMu.Lock()
	if o.live.animRunning && o.live.animStop != nil {
		close(o.live.animStop)
		o.live.animStop = nil
		o.live.animRunning = false
	}
	o.live.animMu.Unlock()
}

// resizeWatcher is implemented by terminal.ANSI (SIGWINCH) and no-ops elsewhere.
type resizeWatcher interface {
	StartResizeWatch(onResize func())
	StopResizeWatch()
}

// startResizeWatchLocked arms SIGWINCH → RefreshSize + forced live redraw.
func (o *Output) startResizeWatchLocked(live LiveSurface) {
	w, ok := live.(resizeWatcher)
	if !ok || o.live == nil || o.live.resizeArmed {
		return
	}
	o.live.resizeArmed = true
	w.StartResizeWatch(func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.closed || o.finished || o.live == nil || !o.live.visible {
			return
		}
		o.signalLiveLocked(true)
	})
}

func (o *Output) stopResizeWatchLocked() {
	if o.live == nil || !o.live.resizeArmed {
		return
	}
	if w, ok := o.live.surface.(resizeWatcher); ok {
		w.StopResizeWatch()
	}
	o.live.resizeArmed = false
}

func (o *Output) spinnerAnimateLoop(stop <-chan struct{}) {
	// Real wall ticker: spinner cadence is independent of the domain clock and
	// of Progress/Phase call rate. Domain clock still selects the glyph frame
	// (FixedClock freezes animation for golden tests).
	t := time.NewTicker(spinnerPeriod)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			o.mu.Lock()
			if o.closed || o.finished || o.live == nil {
				o.stopSpinnerAnimatorLocked()
				o.mu.Unlock()
				return
			}
			// Promote visibility after VisibilityDelay using domain clock.
			if o.live.waitingDelay && o.hasLiveActivityLocked() {
				delay := o.cfg.visibilityDelay
				now := o.cfg.clock.Now()
				if delay <= 0 || now.Sub(o.live.activitySince) >= delay {
					o.live.visible = true
					o.live.waitingDelay = false
					o.renderLiveLocked(true)
				}
				o.mu.Unlock()
				continue
			}
			if !o.live.visible {
				o.stopSpinnerAnimatorLocked()
				o.mu.Unlock()
				return
			}
			if !o.needsSpinnerAnimLocked() {
				o.stopSpinnerAnimatorLocked()
				o.mu.Unlock()
				return
			}
			// Force redraw so time-based spinner glyphs advance.
			o.renderLiveLocked(true)
			o.mu.Unlock()
		}
	}
}

// stampLiveFirstSeenLocked anchors each unresolved task's heartbeat clock to
// the moment it is actually painted in the live region for the first time —
// never to declaration time, so a task declared up-front but not yet visible
// does not appear pre-aged the instant it is finally shown (evo-rec.md
// Problem 9). A no-op once set: the field only ever moves from zero once.
func (o *Output) stampLiveFirstSeenLocked(now time.Time) {
	for _, t := range o.tasks {
		if t.state != Running && t.state != Pending {
			continue
		}
		if t.liveFirstSeenAt.IsZero() {
			t.liveFirstSeenAt = now
		}
	}
}

// activitySince picks the anchor heartbeatSuffix measures elapsed time from:
// ActivityAt when a Phase/Progress call has actually happened, otherwise the
// row's first live-region render (see stampLiveFirstSeenLocked) — so a
// Pending row, which never gets ActivityAt, still ages honestly from the
// moment it became visible rather than never aging at all.
func activitySince(t TaskSnapshot) time.Time {
	if !t.ActivityAt.IsZero() {
		return t.ActivityAt
	}
	return t.LiveFirstSeenAt()
}

// heartbeatSuffix returns " — <elapsed>" once since has gone stale past
// phaseStaleAfter, or "" otherwise (including a zero since — a task that has
// never had Phase/Progress set gets no heartbeat).
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

func (o *Output) debugLiveLocked(line string) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
	}
	if o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
	}
	live.WriteDurable(line)
	if o.live.visible && o.hasLiveActivityLocked() {
		o.renderLiveLocked(true)
	}
}

func (o *Output) finishLiveLocked(final string) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	o.stopSpinnerAnimatorLocked()
	if o.live != nil && o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
		o.live.visible = false
	}
	// H.17 expects a compact final task line, not the full multi-section report.
	live.WriteFinal(strings.TrimRight(final, "\n"))
}

// renderLiveRegion builds the interactive ledger text for the current snapshot.
// now selects spinner frames (inject FixedClock in tests for stable glyphs).
// color applies SGR to glyphs as rows resolve (✓ green, ✗ red, spinner cyan).
func renderLiveRegion(s Snapshot, height, width int, now time.Time, color bool, profile GlyphProfile) string {
	if height <= 0 {
		height = 24
	}
	var b strings.Builder
	spin := spinnerGlyph(now, profile)

	// Prefer collections for multi-task progress display.
	for _, col := range s.Collections {
		writeLiveCollection(&b, col, height, width, spin, color, now, profile)
	}
	for _, t := range s.Tasks {
		writeLiveTaskLine(&b, t, 0, width, spin, color, now, profile)
	}
	return strings.TrimRight(b.String(), "\n")
}

// liveTickerSnapshotLocked is the snapshot the live ticker draws from: every
// root task except one already durably flushed by commitResolvedTaskLocked
// (a never-ran "fact-check" resolution — see its doc comment). Without this
// filter, a task dropped from the ticker onto durable text would reappear on
// the next unrelated redraw and double-print.
func (o *Output) liveTickerSnapshotLocked() Snapshot {
	snap := o.snapshotLocked()
	visible := snap.Tasks[:0]
	for _, t := range snap.Tasks {
		if st := o.taskByRef[t.ID]; st != nil && st.collection == nil && st.coreEmitted {
			continue
		}
		visible = append(visible, t)
	}
	snap.Tasks = visible
	return snap
}

// renderLiveRegionWithDebug builds the live ledger plus optional rolling debug pane (§21.3.2).
func (o *Output) renderLiveRegionWithDebugLocked(width, height int, now time.Time) string {
	color := !o.cfg.noColor
	profile := o.cfg.glyphs
	bodyHeight := height
	if o.cfg.debugPresentation == DebugPresentationPane && len(o.debugRecords) > 0 {
		// Reserve rows for pane heading + visible records before budgeting the body.
		paneRows := debugPaneReservedRows(o.cfg.debugPane, len(o.debugRecords))
		if paneRows >= height {
			paneRows = height - 1
		}
		if paneRows < 0 {
			paneRows = 0
		}
		bodyHeight = height - paneRows
		if bodyHeight < 1 {
			bodyHeight = 1
		}
	}
	body := renderLiveRegion(o.liveTickerSnapshotLocked(), bodyHeight, width, now, color, profile)
	if body == "" && o.armed && len(o.tasks) == 0 && len(o.collections) == 0 {
		body = renderArmedTitleLine(o.cfg.subject, now, color, profile)
	}
	if o.cfg.debugPresentation != DebugPresentationPane || len(o.debugRecords) == 0 {
		return fitLiveRegion(body, width)
	}
	var b strings.Builder
	b.WriteString(body)
	writeDebugPane(&b, o.debugRecords, o.cfg.debugPane, width, color)
	return fitLiveRegion(strings.TrimRight(b.String(), "\n"), width)
}

// renderArmedTitleLine is the honest placeholder painted after arm() when the
// caller has not declared any entity yet — e.g. still parsing config. Falls
// back to a generic label rather than an empty string so the paint stays
// honest (never blank) even before Config.Title is known.
func renderArmedTitleLine(subject string, now time.Time, color bool, profile GlyphProfile) string {
	title := subject
	if title == "" {
		title = "starting"
	}
	return fmt.Sprintf("%s  %s", styleGlyph(spinnerGlyph(now, profile), sgrCyan, color), title)
}

func fitLiveRegion(text string, columns int) string {
	if columns <= 0 {
		columns = defaultWidth
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = txt.TruncateVisible(line, columns)
	}
	return strings.Join(lines, "\n")
}

func writeLiveCollection(b *strings.Builder, col TasksSnapshot, height, width int, spin string, color bool, now time.Time, profile GlyphProfile) {
	done, total := 0, len(col.Tasks)
	for _, t := range col.Tasks {
		if t.State == Done || t.State == Skipped {
			done++
		}
	}
	// Header
	glyph := taskGlyph(col.State, profile)
	if col.State == Failed || anyChildFailed(col) {
		if col.State == Failed {
			glyph = glyphFailedState.render(profile)
		}
	}
	// When any running, animate header spinner (H.20 uses FixedClock → stable ⠋).
	if anyChildRunning(col) || anyChildPendingActive(col) {
		glyph = spin
	}
	headerState := col.State
	if anyChildRunning(col) || anyChildPendingActive(col) {
		headerState = Running
	}
	fmt.Fprintf(b, "%s  %s  %d/%d complete\n",
		styleGlyph(glyph, stateColor(headerState), color), col.Name, done, total)

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
		fmt.Fprintf(b, "   %s  %d not shown\n", dim(glyphOverflow.render(profile), color), omitted)
	}
}

func anyChildRunning(col TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == Running {
			return true
		}
	}
	return false
}

func anyChildFailed(col TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == Failed {
			return true
		}
	}
	return false
}

// anyChildPendingActive reports whether the collection has any unresolved
// child — Running, or Pending regardless of Phase. A Pending child that
// never called Phase is the ordinary case (Phase/Progress are the only
// Running promoters), so requiring Phase here used to leave an all-Pending
// or Pending-tailed collection's header frozen on derivedState's static
// Incomplete glyph (evo-rec.md Problem 9).
func anyChildPendingActive(col TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == Running || t.State == Pending {
			return true
		}
	}
	return false
}

func selectLiveChildren(tasks []TaskSnapshot, max int) (selected []TaskSnapshot, omitted int) {
	if len(tasks) <= max {
		return tasks, 0
	}
	// Priority: failed, warning, active(running), pending, successful.
	rank := func(t TaskSnapshot) int {
		switch t.State {
		case Failed:
			return 0
		case Warning:
			return 1
		case Running:
			return 2
		case Pending:
			return 3
		case Done, Skipped:
			return 4
		default:
			return 5
		}
	}
	// Stable: collect by rank preserving declaration order within class.
	var buckets [6][]TaskSnapshot
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

func writeLiveTaskLine(b *strings.Builder, t TaskSnapshot, indent, width int, spin string, color bool, now time.Time, profile GlyphProfile) {
	pad := ""
	if indent > 0 {
		pad = "   "
	}
	glyph := taskGlyph(t.State, profile)
	if t.State == Running {
		glyph = spin
	}
	g := styleGlyph(glyph, stateColor(t.State), color)
	// Child rows: indent + glyph + two spaces + name padded to 9 + two spaces + detail.
	// Produces stable columns: "react" and "sharp" share alignment; "esbuild" fills the field.
	nameField := t.Name
	if indent > 0 {
		nameField = padRight(t.Name, 9)
	}
	switch {
	case t.State == Done && t.Progress.Kind == BytesKind:
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, formatBytes(t.Progress.Completed))
	case t.State == Done && t.Summary != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, dim(t.Summary, color))
	case t.State == Running && t.Progress.Kind == BytesKind && t.Progress.Total > 0:
		detail := formatByteProgressFixed(t.Progress.Completed, t.Progress.Total)
		detail = progressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " + detail
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, detail)
	case t.State == Running && t.Progress.Kind == Determinate && t.Progress.Total > 0:
		count := fmt.Sprintf("%d/%d", t.Progress.Completed, t.Progress.Total)
		// Narrow terminals degrade by dropping decoration (the bar) before
		// information (count, name) — evo-rec.md Problem 16/26's compact
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
			// "Color and style demotions").
			detail = detail + "  " + t.Phase + heartbeatSuffix(now, activitySince(t))
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, detail)
	case t.State == Running && (t.Progress.Kind == Indeterminate || t.Phase != "" || (t.Progress.Kind == Determinate && t.Progress.Total <= 0)):
		// Indeterminate, or Determinate with nothing to divide by (Total<=0,
		// e.g. Progress(0,0)): spinner glyph + phase (or generic working) —
		// folded together because neither has a renderable count/bar, so both
		// need the same "is this actually still moving?" heartbeat.
		phase := t.Phase
		if phase == "" {
			phase = "working…"
		}
		phase += heartbeatSuffix(now, activitySince(t))
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, phase)
	case t.State == Running && t.Phase != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, t.Phase)
	case t.State == Pending:
		// A Pending row left on screen past phaseStaleAfter is exactly as
		// static as a stalled Running one — same heartbeat, dim (subordinate:
		// nothing is happening yet) rather than the diagnostic-intensity
		// phase text a Running row gets.
		if hb := heartbeatSuffix(now, activitySince(t)); hb != "" {
			fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, dim("waiting"+hb, color))
		} else {
			fmt.Fprintf(b, "%s%s  %s\n", pad, g, strings.TrimRight(nameField, " "))
		}
	case t.State == Failed:
		msg := t.Summary
		if msg == "" && len(t.Problems) > 0 {
			msg = t.Problems[0].Summary
		}
		// release-gate round 8 finding 4: a task that failed mid-loop still
		// carries the in-flight count it had when Fail was called — render
		// it in the same position a Running row shows it (right after the
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
	case t.State == Warning:
		msg := t.Summary
		if msg == "" && len(t.Problems) > 0 {
			msg = t.Problems[0].Summary
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, msg)
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
