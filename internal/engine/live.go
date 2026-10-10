package engine

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render/live"
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
	surface LiveSurface
	visible bool
	// quiesced counts the open Suspend windows (guarded by o.mu): while any
	// is open nothing is painted, and no signal makes the region visible.
	quiesced int

	// paintMu serializes writes to the surface and guards what describes
	// them. Lock order is o.mu, then paintMu; a paint never takes o.mu
	// while holding it.
	paintMu      sync.Mutex
	lastLiveText string
	liveActive   bool
	floorSeq     uint64 // frames at or below it were snapshotted before a clear
	paintedSeq   uint64 // the newest frame that reached paint

	// seq numbers frames in snapshot order; guarded by o.mu.
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

	// forced meters the render work forced paints spent in the current
	// frame interval.
	forced renderBudget
}

func (o *Output) liveLocked() LiveSurface {
	if o.cfg.plain {
		return nil
	}
	return asLive(o.cfg.terminal)
}

// signalLiveLocked marks that interactive presentation may need a redraw.
// force=true bypasses frame-rate coalescing only (not VisibilityDelay).
// VisibilityDelay withholds the first live paint after activity starts, except
// a newly declared Running task always paints (FP-005: spinner before check).
func (o *Output) signalLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
		o.startResizeWatchLocked(live)
	}
	if o.live.quiesced > 0 {
		return
	}
	now := o.cfg.clock.Now()
	if !o.visibilitySettledLocked(now) && o.hasLiveActivityLocked() {
		if o.live.activitySince.IsZero() {
			o.live.activitySince = now
		}
		delay := o.cfg.visibilityDelay
		// delay <= 0 means immediate (tests use visibilityDelay(0)). A newly
		// declared Running task bypasses the delay so the first frame is a
		// spinner, not a popup-complete check (FP-005).
		if delay <= 0 || now.Sub(o.live.activitySince) >= delay || o.hasUnpaintedRunningLocked() || o.armedTitleLiveLocked() {
			o.live.visible = true
			o.live.waitingDelay = false
		} else {
			o.live.waitingDelay = true
			// Wall ticker re-checks delay; fixedClock tests re-enter after Advance.
			o.ensureSpinnerAnimatorLocked()
			return
		}
	}
	if !o.live.visible {
		return
	}
	lastRender := o.live.lastRender.Load()
	if lastRender != nil && len(o.tasks) > forcedRenderRowsPerInterval {
		// Past the forced budget's row count the animator owns painting: it
		// ticks every spinner period while anything is unresolved, so workers
		// never render inline. The first paint and every direct
		// renderLiveLocked caller (finish, suspend, durable redraw) stay
		// synchronous. The gate counts Tasks, not time, so it is deterministic.
		o.live.pendingRedraw.Store(true)
		o.ensureSpinnerAnimatorLocked()
		return
	}
	minGap := time.Second / time.Duration(max(1, o.cfg.maxFrameRate))
	if force && !o.live.forced.allow(now, minGap, len(o.tasks)) {
		force = false
	}
	if !force && lastRender != nil {
		if now.Sub(*lastRender) < minGap {
			// The animator paints the coalesced change on its next tick.
			o.live.pendingRedraw.Store(true)
			o.ensureSpinnerAnimatorLocked()
			return
		}
	}
	o.renderLiveLocked(force)
	o.ensureSpinnerAnimatorLocked()
}

// visibilitySettledLocked reports whether VisibilityDelay can no longer
// withhold a paint: the region is visible and the delay since activity
// began has elapsed. Past that point the activity scan decides nothing,
// and skipping it keeps each signal from walking every Task.
func (o *Output) visibilitySettledLocked(now time.Time) bool {
	return o.live.visible && !o.live.activitySince.IsZero() &&
		(o.cfg.visibilityDelay <= 0 || now.Sub(o.live.activitySince) >= o.cfg.visibilityDelay)
}

// holdRunningPaint keeps a Running row on screen for one spinner period so a
// fast bind cannot resolve in the same tick the spinner was painted — the
// viewer would otherwise see a popup-complete check (FP-005). Skipped when
// the clock is not the wall clock (tests inject Clock) or the surface is
// not an interactive live terminal.
func (o *Output) holdRunningPaint(id string) {
	o.mu.Lock()
	if _, ok := o.cfg.clock.(systemClock); !ok {
		o.mu.Unlock()
		return
	}
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		o.mu.Unlock()
		return
	}
	st := o.taskStates[id]
	if st == nil || st.node.Rec.State() != Running {
		o.mu.Unlock()
		return
	}
	seen := st.liveFirstSeenAt
	o.mu.Unlock()
	wait := txt.SpinnerPeriod
	if !seen.IsZero() {
		wait = txt.SpinnerPeriod - wall.Since(seen)
	}
	if wait > 0 {
		wall.Sleep(wait)
	}
}

func (o *Output) hasUnpaintedRunningLocked() bool {
	for _, t := range o.tasks {
		if t.node.Rec.State() == Running && t.liveFirstSeenAt.IsZero() {
			return true
		}
	}
	return false
}

func (o *Output) hasLiveActivityLocked() bool {
	for _, t := range o.tasks {
		// Pending counts too: a declared task renders its named "○" row
		// immediately (evo-rec.md "predeclare Tasks; ... others named
		// idle") — VisibilityDelay withholds the *spinner flash* for
		// near-instant work, not the fact that a task now exists.
		if t.node.Rec.State() == Running || t.node.Rec.State() == Pending || t.node.Rec.Phase() != "" {
			return true
		}
		if t.node.Rec.Progress().Kind == Determinate || t.node.Rec.Progress().Kind == BytesKind {
			if t.node.Rec.State() == Running || t.node.Rec.State() == Done || t.node.Rec.State() == Failed {
				return true
			}
		}
	}
	// Armed-but-empty: Init promised a paint before any entity exists. Once
	// the first Task/Tasks is declared, its own state drives activity.
	return o.armedTitleLiveLocked()
}

// armedTitleLiveLocked is the title-only live spinner painted by arm() before
// any Task exists. hasLiveActivityLocked and needsSpinnerAnimLocked must agree
// on this predicate — otherwise the header spinner is shown and then frozen.
func (o *Output) armedTitleLiveLocked() bool {
	return o.armed && len(o.tasks) == 0 && len(o.collections) == 0
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
	if live == nil || o.live == nil || !o.live.visible || o.live.quiesced > 0 {
		return
	}
	o.paintFrameLocked(live)
}

// needsSpinnerAnimLocked reports whether any live row should keep ticking: the
// armed title-only spinner, or a Pending/Running task still rendered in the
// current frame — not a standalone task already flushed to durable text and
// dropped from the ticker (see liveSnapshotLocked's matching filter).
// Counting only Running here used to let an all-Pending frame freeze forever
// (evo-rec.md Problem 9). Omitting the armed title froze "⠦  zq" during
// Init-to-first-Task work (ensureReady, fingerprint, Inspect).
func (o *Output) needsSpinnerAnimLocked() bool {
	if o.armedTitleLiveLocked() {
		return true
	}
	for _, col := range o.collections {
		if census := col.currentCensus(); census.running+census.pending > 0 {
			return true
		}
	}
	for _, t := range o.rootTasks {
		if (t.node.Rec.State() == Running || t.node.Rec.State() == Pending) && !t.coreEmitted {
			return true
		}
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
	// A running animator re-checks needsSpinnerAnimLocked on every tick and
	// stops itself, so skip that O(n) scan on each signal while it runs.
	o.live.animMu.Lock()
	running := o.live.animRunning
	o.live.animMu.Unlock()
	if running {
		return
	}
	// Waiting for delay: keep a ticker so we can paint when the threshold elapses.
	switch {
	case o.live.waitingDelay:
		// fall through to start animator
	case !o.live.visible:
		return
	case !o.needsSpinnerAnimLocked() && !o.live.pendingRedraw.Load():
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
	// Real wall timer: spinner cadence is independent of the domain clock and
	// of Progress/Phase call rate. Domain clock still selects the glyph frame
	// (fixedClock freezes animation for golden tests).
	t := wall.NewTimer(untilNextSpinnerSlot(wall.Now()))
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			t.Reset(untilNextSpinnerSlot(wall.Now()))
			o.mu.Lock()
			next := o.animatorTickLocked()
			if next == tickPaint || next == tickFlush {
				o.paintFrameUnlocked(o.live.surface)
			}
			o.mu.Unlock()
			if next == tickStop || next == tickFlush {
				return
			}
		}
	}
}

// animatorStep is what one animator tick decided.
type animatorStep int

const (
	tickContinue animatorStep = iota // nothing to paint; keep ticking
	tickStop                         // nothing to paint; the animator has stopped
	tickPaint                        // paint a frame, keep ticking
	tickFlush                        // paint the last coalesced change; the animator has stopped
)

// animatorTickLocked decides one tick: it settles visibility and stops the
// animator when nothing is left to animate, and skips a glyph-only repaint
// while an overloaded surface was written too recently to need it.
func (o *Output) animatorTickLocked() animatorStep {
	if o.closed || o.finished || o.live == nil {
		o.stopSpinnerAnimatorLocked()
		return tickStop
	}
	if o.live.quiesced > 0 {
		return tickContinue
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
		return tickContinue
	}
	if !o.live.visible {
		o.stopSpinnerAnimatorLocked()
		return tickStop
	}
	if !o.needsSpinnerAnimLocked() {
		o.stopSpinnerAnimatorLocked()
		// Paint the last coalesced change before going quiet, so
		// the frame on screen is never older than the run.
		if o.live.pendingRedraw.Load() {
			return tickFlush
		}
		return tickStop
	}
	if !o.live.pendingRedraw.Load() && o.live.shouldSkipGlyph() {
		o.live.skippedGlyph.Add(1)
		return tickContinue
	}
	// Force redraw so time-based spinner glyphs advance.
	return tickPaint
}

// spinnerSlotSettle is how far past a glyph-slot boundary the animator
// wakes, so the paint reads the new slot despite wall-clock slew.
const spinnerSlotSettle = time.Millisecond

// untilNextSpinnerSlot is the wait from now to just past the next boundary
// of txt.SpinnerGlyph's slots. A free-running ticker has an arbitrary phase
// against those slots: two ticks could read the same glyph, the identical
// frame would be skipped, and a quiet Running row would sit stale for two
// periods. Waking once per slot makes every wake a changed frame.
func untilNextSpinnerSlot(now time.Time) time.Duration {
	period := int64(txt.SpinnerPeriod)
	return time.Duration(period-now.UnixNano()%period) + spinnerSlotSettle
}

// stampLiveFirstSeen anchors an unresolved task's heartbeat clock to the
// moment it is actually painted in the live region for the first time —
// never to declaration time, so a task declared up-front but not yet visible
// does not appear pre-aged the instant it is finally shown (evo-rec.md
// Problem 9). A no-op once set: the field only ever moves from zero once.
// liveSnapshotLocked calls it on every Task as it builds a frame.
func (t *taskState) stampLiveFirstSeen(now time.Time) {
	if t.unstampedIn(t.node.Rec.State()) {
		t.liveFirstSeenAt = now
		t.markFiling()
		t.censusStamped()
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
	o.live.clear(live)
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
	if o.live != nil && o.live.clear(live) {
		o.live.visible = false
	}
	// H.17 expects a compact final task line, not the full multi-section report.
	live.WriteFinal(strings.TrimRight(final, "\n"))
}

// liveSnapshotLocked is what a live frame of rows rows draws from: the
// effect sections, and every collection and standalone root Task as far
// as the frame could show it (live.LiveChildren), so building a frame
// snapshots the rows on screen rather than every Task in the run. A root
// Task already durably flushed by commitResolvedTaskLocked (a never-ran
// "fact-check" resolution — see its doc comment) is left out: it would
// otherwise reappear on the next unrelated redraw and double-print.
func (o *Output) liveSnapshotLocked(rows int, now time.Time) Snapshot {
	var s Snapshot
	s.Changes = o.rec.ChangeSnapshots()
	s.Plans = o.rec.PlanSnapshots()
	cols := liveCollections(o.collections, rows, now)
	s.Collections = cols.Kept()
	s = core.WithRootCollectionTally(s, cols.Tally())
	root := live.NewLiveChildren("", rows)
	for _, t := range o.rootTasks {
		t.stampLiveFirstSeen(now)
		view := t.view()
		if t.coreEmitted || o.heldBackAsNoOpLocked(view) {
			continue
		}
		if root.Admit(&view) {
			root.Keep(t.snapshot())
		}
	}
	tasks := root.Collection(TasksSnapshot{})
	s.Tasks = tasks.Tasks
	if tally, ok := core.ChildTallyOf(tasks); ok {
		s = core.WithRootTally(s, tally)
	}
	return s
}
