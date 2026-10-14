package live

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/internal/clock"
	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// wall times frames and drives the animator: spinner cadence is
// independent of the domain clock and of the rate work reports at.
var wall = clock.System()

// Engine paints one run's live region on an interactive Surface. It decides
// when a frame is due (VisibilityDelay, the frame-rate cap, the forced
// budget), animates spinners on a wall timer, follows resizes, and keeps a
// frame snapshotted before a clear from ever landing after it.
//
// Every method runs under the run's lock (Config.Lock) unless its doc says
// otherwise; the animator and the resize watch take that lock themselves.
type Engine struct {
	cfg Config

	visible bool
	// quiesced counts the open windows (Open): while any is open nothing
	// is painted, and no signal makes the region visible.
	quiesced int
	// held is the durable text that settled while a window was open, in
	// arrival order: the window owns the terminal, so it prints once the
	// last window closes.
	held []string

	// paintMu serializes writes to the surface and guards what describes
	// them. Lock order is the run's lock, then paintMu; a paint never takes
	// the run's lock while holding it.
	paintMu    sync.Mutex
	lastText   string
	onScreen   bool
	floorSeq   uint64 // frames at or below it were snapshotted before a clear
	paintedSeq uint64 // the newest frame that reached paint

	// seq numbers frames in snapshot order.
	seq uint64

	// lastRender is the domain time of the last write (nil: never painted),
	// lastWrite the wall time it began.
	lastRender    atomic.Pointer[time.Time]
	lastWrite     atomic.Pointer[time.Time]
	pendingRedraw atomic.Bool

	// meter and skippedGlyph back the animator's overload backpressure.
	meter        frameMeter
	skippedGlyph atomic.Int64

	// activitySince is set when live activity first appears; VisibilityDelay
	// withholds the first paint until the domain clock passes it.
	activitySince time.Time
	waitingDelay  bool

	// The animator ticks spinners while anything is unresolved, so
	// indeterminate rows animate when nothing reports, and promotes
	// visibility once VisibilityDelay elapses.
	animMu      sync.Mutex
	animRunning bool
	animStop    chan struct{}

	// resizeArmed is set while the surface's resize watch is registered.
	resizeArmed bool

	// forced meters the render work forced paints spent this frame interval.
	forced renderBudget
}

// Config is what an Engine paints and how.
type Config struct {
	Surface Surface
	Board   *Board
	Scene   Scene
	// Lock is the run's lock: it guards the Board and every Engine method.
	Lock sync.Locker
	// Clock is the domain clock: it selects spinner glyphs and measures
	// VisibilityDelay.
	Clock clock.Clock
	// VisibilityDelay withholds the first paint after activity starts;
	// zero or less paints at once.
	VisibilityDelay time.Duration
	// MaxFrameRate caps unforced paints per second.
	MaxFrameRate int
	Style        project.Style
	Subject      string
}

// Scene is what an Engine reads of the run around it, under the run's lock.
type Scene interface {
	// Ended reports the run closed or finished: no frame paints and no
	// tick runs again.
	Ended() bool
	// Pane is the debug pane a frame width columns wide and height rows
	// tall draws under its rows, and the rows it reserves; "" for none.
	Pane(width, height int) (text string, rows int)
	// Resized tells the run the surface is now width columns wide.
	Resized(width int)
}

// NewEngine starts an Engine that paints nothing until signalled.
func NewEngine(cfg Config) *Engine { return &Engine{cfg: cfg} }

// Interactive reports whether e paints a surface that is interactive now.
// A nil Engine is not.
func (e *Engine) Interactive() bool { return e != nil && e.cfg.Surface.IsInteractive() }

// Signal marks that the region may need a redraw. force bypasses the
// frame-rate cap only, never VisibilityDelay, except that a newly declared
// Running Task always paints (FP-005: spinner before check).
func (e *Engine) Signal(force bool) {
	if !e.Interactive() {
		return
	}
	e.watchResize()
	if e.quiesced > 0 {
		return
	}
	now := e.cfg.Clock.Now()
	if !e.visibilitySettled(now) && e.cfg.Board.Active() && !e.promote(now) {
		return
	}
	if !e.visible {
		return
	}
	lastRender := e.lastRender.Load()
	if lastRender != nil && e.cfg.Board.Rows() > forcedRenderRowsPerInterval {
		// Past the forced budget's row count the animator owns painting: it
		// ticks every spinner period while anything is unresolved, so
		// workers never render inline. The gate counts Tasks, not time, so
		// it is deterministic.
		e.pendingRedraw.Store(true)
		e.ensureAnimator()
		return
	}
	minGap := time.Second / time.Duration(max(1, e.cfg.MaxFrameRate))
	if force && !e.forced.allow(now, minGap, e.cfg.Board.Rows()) {
		force = false
	}
	if !force && lastRender != nil && now.Sub(*lastRender) < minGap {
		// The animator paints the coalesced change on its next tick.
		e.pendingRedraw.Store(true)
		e.ensureAnimator()
		return
	}
	e.Render()
	e.ensureAnimator()
}

// promote makes the region visible once VisibilityDelay has passed since
// activity began, or at once for an unpainted Running Task or the armed
// title, and reports whether it did. Until then a ticker re-checks.
func (e *Engine) promote(now time.Time) bool {
	if e.activitySince.IsZero() {
		e.activitySince = now
	}
	delay := e.cfg.VisibilityDelay
	if delay <= 0 || now.Sub(e.activitySince) >= delay || e.cfg.Board.UnpaintedRunning() || e.cfg.Board.ArmedTitle() {
		e.visible = true
		e.waitingDelay = false
		return true
	}
	e.waitingDelay = true
	e.ensureAnimator()
	return false
}

// visibilitySettled reports whether VisibilityDelay can no longer withhold
// a paint: the region is visible and the delay since activity began has
// elapsed. Past that point the activity scan decides nothing, and skipping
// it keeps each signal from walking every Task.
func (e *Engine) visibilitySettled(now time.Time) bool {
	return e.visible && !e.activitySince.IsZero() &&
		(e.cfg.VisibilityDelay <= 0 || now.Sub(e.activitySince) >= e.cfg.VisibilityDelay)
}

// Render paints a frame now unless the region is hidden or a window is
// open.
func (e *Engine) Render() {
	if e == nil || !e.visible || e.quiesced > 0 {
		return
	}
	e.paintLocked()
}

// Show makes the region visible and paints it.
func (e *Engine) Show() {
	e.visible = true
	e.Render()
}

// Refresh brings the region in line with the board after a row left it for
// scrollback: repainted while anything is active, cleared and stilled when
// nothing is.
func (e *Engine) Refresh() {
	if !e.Interactive() {
		return
	}
	if e.cfg.Board.Active() {
		e.Show()
		return
	}
	if e.Clear() {
		e.visible = false
		e.stopAnimator()
	}
}

// Durable writes text above the region and redraws it in the same locked
// sequence, so the region never sits blank or stale between the write and
// the next tick (evo-rec.md #12). A region still on screen is cleared even
// when the surface stopped reporting itself interactive (terminal.ANSI
// does on a short write), so durable text never lands on a stale frame. It
// reports whether the surface took the text.
func (e *Engine) Durable(text string) bool {
	if e == nil {
		return false
	}
	e.Clear()
	if !e.Interactive() {
		return false
	}
	e.cfg.Surface.WriteDurable(text)
	if e.visible && e.cfg.Board.Active() {
		e.Render()
	}
	return true
}

// Finish clears the region and writes final, the compact last word, in its
// place (H.17).
func (e *Engine) Finish(final string) {
	if !e.Interactive() {
		return
	}
	e.stopAnimator()
	if e.Clear() {
		e.visible = false
	}
	e.cfg.Surface.WriteFinal(strings.TrimRight(final, "\n"))
}

// Stop ends the animator and the resize watch for good.
func (e *Engine) Stop() {
	if e == nil {
		return
	}
	e.stopAnimator()
	if !e.resizeArmed {
		return
	}
	if w, ok := e.cfg.Surface.(resizeWatcher); ok {
		w.StopResizeWatch()
	}
	e.resizeArmed = false
}

// HoldRunning keeps the Task id's Running row on screen for one spinner
// period, so a fast bind cannot resolve in the tick its spinner was painted
// and show a popup-complete check (FP-005). It skips a domain clock that is
// not the wall clock (tests inject one) and a surface that is not
// interactive. It takes the run's lock itself; call it without.
func (e *Engine) HoldRunning(id record.TaskID) {
	if e == nil {
		return
	}
	e.cfg.Lock.Lock()
	_, isWall := e.cfg.Clock.(clock.Wall)
	seen, running := e.cfg.Board.runningSince(id)
	hold := isWall && e.Interactive() && running
	e.cfg.Lock.Unlock()
	if !hold {
		return
	}
	wait := project.SpinnerPeriod
	if !seen.IsZero() {
		wait -= wall.Since(seen)
	}
	if wait > 0 {
		wall.Sleep(wait)
	}
}

// watchResize arms the surface's resize watch once: a resize repaints at
// once.
func (e *Engine) watchResize() {
	w, ok := e.cfg.Surface.(resizeWatcher)
	if !ok || e.resizeArmed {
		return
	}
	e.resizeArmed = true
	w.StartResizeWatch(func() {
		e.cfg.Lock.Lock()
		defer e.cfg.Lock.Unlock()
		if e.cfg.Scene.Ended() || !e.visible {
			return
		}
		e.Signal(true)
	})
}
