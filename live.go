package evo

import (
	"fmt"
	"strings"
	"time"
)

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
	text := renderLiveRegion(o.snapshotLocked(), live.Columns(), live.Rows())
	if !force && text == o.live.lastLiveText {
		return
	}
	live.WriteLive(text)
	o.live.lastLiveText = text
	o.live.lastRender = o.cfg.clock.Now()
	o.live.liveActive = true
	o.live.pendingRedraw = false
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
	if o.live != nil && o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
	}
	// H.17 expects a compact final task line, not the full multi-section report.
	live.WriteFinal(strings.TrimRight(final, "\n"))
}

// renderLiveRegion builds the interactive ledger text for the current snapshot.
func renderLiveRegion(s Snapshot, width, height int) string {
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = 24
	}
	var b strings.Builder

	// Prefer collections for multi-task progress display.
	for _, col := range s.Collections {
		writeLiveCollection(&b, col, height)
	}
	for _, t := range s.Tasks {
		writeLiveTaskLine(&b, t, 0)
	}
	for _, it := range s.Items {
		if it.State == Running || it.State == Pending {
			fmt.Fprintf(&b, "%s  %s\n", itemGlyph(it.State), it.Name)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeLiveCollection(b *strings.Builder, col TasksSnapshot, height int) {
	done, total := 0, len(col.Tasks)
	for _, t := range col.Tasks {
		if t.State == Done || t.State == Skipped {
			done++
		}
	}
	// Header
	glyph := taskGlyph(col.State)
	if col.State == Running || anyChildRunning(col) {
		glyph = "⠋"
	}
	if col.State == Failed || anyChildFailed(col) {
		// keep failed glyph on collection if failed
		if col.State == Failed {
			glyph = "✗"
		}
	}
	// When any running, show spinner on header even if derived failed? Spec H.20 shows spinner with 1/3 complete.
	if anyChildRunning(col) || anyChildPendingActive(col) {
		glyph = "⠋"
	}
	fmt.Fprintf(b, "%s  %s  %d/%d complete\n", glyph, col.Name, done, total)

	// Select children by severity under height budget.
	// Budget: height includes header; leave room for omission line.
	maxChildRows := height - 2 // header + possible omission
	if maxChildRows < 1 {
		maxChildRows = 1
	}
	selected, omitted := selectLiveChildren(col.Tasks, maxChildRows)
	for _, t := range selected {
		writeLiveTaskLine(b, t, 1)
	}
	if omitted > 0 {
		fmt.Fprintf(b, "   …  %d not shown\n", omitted)
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

func writeLiveTaskLine(b *strings.Builder, t TaskSnapshot, indent int) {
	pad := ""
	if indent > 0 {
		pad = "   "
	}
	glyph := taskGlyph(t.State)
	if t.State == Running {
		glyph = "⠋"
	}
	// Child rows: indent + glyph + two spaces + name padded to 9 + two spaces + detail.
	// Produces stable columns: "react" and "sharp" share alignment; "esbuild" fills the field.
	nameField := t.Name
	if indent > 0 {
		nameField = padRight(t.Name, 9)
	}
	switch {
	case t.State == Done && t.Progress.Kind == BytesKind:
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, glyph, nameField, formatBytes(t.Progress.Completed))
	case t.State == Done && t.Summary != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, glyph, nameField, t.Summary)
	case t.State == Running && t.Progress.Kind == BytesKind:
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, glyph, nameField, formatByteProgressFixed(t.Progress.Completed, t.Progress.Total))
	case t.State == Running && t.Phase != "":
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, glyph, nameField, t.Phase)
	case t.State == Failed:
		msg := t.Summary
		if msg == "" && len(t.Problems) > 0 {
			msg = t.Problems[0].Summary
		}
		if msg != "" {
			fmt.Fprintf(b, "%s%s  %s  %s\n", pad, glyph, nameField, msg)
		} else {
			fmt.Fprintf(b, "%s%s  %s\n", pad, glyph, strings.TrimRight(nameField, " "))
		}
	case t.State == Warning:
		msg := t.Summary
		if msg == "" && len(t.Problems) > 0 {
			msg = t.Problems[0].Summary
		}
		fmt.Fprintf(b, "%s%s  %s  %s\n", pad, glyph, nameField, msg)
	default:
		fmt.Fprintf(b, "%s%s  %s\n", pad, glyph, strings.TrimRight(nameField, " "))
	}
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
