package evo

import (
	"strings"
	"sync"
	"time"

	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
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
	t := time.NewTicker(txt.SpinnerPeriod)
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
	body := render.LiveRegion(o.liveTickerSnapshotLocked(), bodyHeight, width, now, color, profile)
	if body == "" && o.armed && len(o.tasks) == 0 && len(o.collections) == 0 {
		body = render.ArmedTitleLine(o.cfg.subject, now, color, profile)
	}
	if o.cfg.debugPresentation != DebugPresentationPane || len(o.debugRecords) == 0 {
		return render.FitLiveRegion(body, width)
	}
	var b strings.Builder
	b.WriteString(body)
	writeDebugPane(&b, o.debugRecords, o.cfg.debugPane, width, color)
	return render.FitLiveRegion(strings.TrimRight(b.String(), "\n"), width)
}
