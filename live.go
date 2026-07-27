package evo

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Braille spinner sequence (common CLI convention).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerPeriod is the wall-clock duration between spinner frame advances.
const spinnerPeriod = 80 * time.Millisecond

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

	// anim drives independent spinner ticks while any task is Running,
	// so indeterminate rows animate even when determinate bars are idle.
	animMu      sync.Mutex
	animRunning bool
	animStop    chan struct{}
}

func (o *Output) liveLocked() LiveSurface {
	if o.cfg.nonInteractive || o.cfg.plain {
		return nil
	}
	return asLive(o.cfg.terminal)
}

// signalLiveLocked marks that interactive presentation may need a redraw.
// force=true bypasses frame-rate coalescing (terminal outcomes, debug, finish).
func (o *Output) signalLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
	}
	// Activity (phase/progress) forces visibility immediately.
	// Instant Done without prior running keeps invisible until final.
	if o.hasLiveActivityLocked() {
		o.live.visible = true
	}
	if !o.live.visible {
		return
	}
	now := o.cfg.clock.Now()
	if !force && o.live.lastRender.IsZero() == false {
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
		if t.state == Running || t.phase != "" {
			return true
		}
		if t.progress.Kind == Determinate || t.progress.Kind == BytesKind {
			if t.state == Running || t.state == Done || t.state == Failed || t.state == Warning {
				return true
			}
		}
	}
	for _, it := range o.items {
		if it.state == Running {
			return true
		}
	}
	return false
}

func (o *Output) renderLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || o.live == nil || !o.live.visible {
		return
	}
	now := o.cfg.clock.Now()
	text := o.renderLiveRegionWithDebugLocked(live.Columns(), live.Rows(), now)
	if !force && text == o.live.lastLiveText {
		return
	}
	live.WriteLive(text)
	o.live.lastLiveText = text
	o.live.lastRender = now
	o.live.liveActive = true
	o.live.pendingRedraw = false
}

// needsSpinnerAnimLocked reports whether any live row should keep ticking.
func (o *Output) needsSpinnerAnimLocked() bool {
	for _, t := range o.tasks {
		if t.state == Running {
			return true
		}
	}
	for _, it := range o.items {
		if it.state == Running {
			return true
		}
	}
	return false
}

// ensureSpinnerAnimatorLocked starts a background tick that re-renders the live
// region so indeterminate spinners advance without waiting for Progress calls.
func (o *Output) ensureSpinnerAnimatorLocked() {
	if o.live == nil || !o.live.visible || o.finished || o.closed {
		return
	}
	if !o.needsSpinnerAnimLocked() {
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
			if o.closed || o.finished || o.live == nil || !o.live.visible {
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

// spinnerGlyph picks a braille frame from the clock so FixedClock freezes it.
func spinnerGlyph(now time.Time) string {
	if len(spinnerFrames) == 0 {
		return "⠋"
	}
	ns := now.UnixNano()
	if ns < 0 {
		ns = -ns
	}
	i := int(ns/int64(spinnerPeriod)) % len(spinnerFrames)
	return spinnerFrames[i]
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
func renderLiveRegion(s Snapshot, width, height int, now time.Time, color bool) string {
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = 24
	}
	var b strings.Builder
	spin := spinnerGlyph(now)

	// Prefer collections for multi-task progress display.
	for _, col := range s.Collections {
		writeLiveCollection(&b, col, height, spin, color)
	}
	for _, t := range s.Tasks {
		writeLiveTaskLine(&b, t, 0, spin, color)
	}
	for _, it := range s.Items {
		if it.State == Running || it.State == Pending {
			g := itemGlyph(it.State)
			if it.State == Running {
				g = spin
			}
			fmt.Fprintf(&b, "%s  %s\n", styleGlyph(g, stateColor(it.State), color), it.Name)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderLiveRegionWithDebug builds the live ledger plus optional rolling debug pane (§21.3.2).
func (o *Output) renderLiveRegionWithDebugLocked(width, height int, now time.Time) string {
	color := !o.cfg.noColor
	body := renderLiveRegion(o.snapshotLocked(), width, height, now, color)
	if o.cfg.debugPresentation != DebugPresentationPane || len(o.debugRecords) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString(body)
	// Budget: leave room for pane; parent already applied height to tasks.
	writeDebugPane(&b, o.debugRecords, o.cfg.debugPane, color)
	return strings.TrimRight(b.String(), "\n")
}

func writeLiveCollection(b *strings.Builder, col TasksSnapshot, height int, spin string, color bool) {
	done, total := 0, len(col.Tasks)
	for _, t := range col.Tasks {
		if t.State == Done || t.State == Skipped {
			done++
		}
	}
	// Header
	glyph := taskGlyph(col.State)
	if col.State == Failed || anyChildFailed(col) {
		if col.State == Failed {
			glyph = "✗"
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
		writeLiveTaskLine(b, t, 1, spin, color)
	}
	if omitted > 0 {
		fmt.Fprintf(b, "   %s  %d not shown\n", dim("…", color), omitted)
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

func anyChildPendingActive(col TasksSnapshot) bool {
	for _, t := range col.Tasks {
		if t.State == Running || (t.State == Pending && t.Phase != "") {
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

func writeLiveTaskLine(b *strings.Builder, t TaskSnapshot, indent int, spin string, color bool) {
	pad := ""
	if indent > 0 {
		pad = "   "
	}
	glyph := taskGlyph(t.State)
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
		detail := progressBar(t.Progress.Completed, t.Progress.Total, 12) + "  " +
			fmt.Sprintf("%d/%d", t.Progress.Completed, t.Progress.Total)
		if t.Phase != "" {
			detail = detail + "  " + t.Phase
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, detail)
	case t.State == Running && (t.Progress.Kind == Indeterminate || t.Phase != ""):
		// Indeterminate: spinner glyph + phase (or generic working).
		phase := t.Phase
		if phase == "" {
			phase = "working…"
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, dim(phase, color))
	case t.State == Running && t.Phase != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, dim(t.Phase, color))
	case t.State == Failed:
		msg := t.Summary
		if msg == "" && len(t.Problems) > 0 {
			msg = t.Problems[0].Summary
		}
		if msg != "" {
			fmt.Fprintf(b, "%s%s  %s  %s\n", pad, g, nameField, msg)
		} else {
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
