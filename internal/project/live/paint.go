package live

import (
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// frame is a live region as the run's lock saw it: everything render needs,
// owned by the frame. Its snapshot is built from owned copies, so rendering
// it after the lock is released cannot race a mutating Task.
type frame struct {
	surface       Surface
	seq           uint64
	snap          record.Snapshot
	width, height int
	now           time.Time
	style         project.Style
	subject       string
	armedTitle    bool
	pane          string
}

// snapshot captures the region for the surface and stamps it with the next
// paint sequence number.
func (e *Engine) snapshot() frame {
	surface := e.cfg.Surface
	if r, ok := surface.(sizeRefresher); ok {
		r.RefreshSize()
	}
	cols, rows := surface.Columns(), surface.Rows()
	if cols > 0 {
		e.cfg.Scene.Resized(cols)
	}
	f := e.frameAt(cols, rows, e.cfg.Clock.Now())
	e.seq++
	f.seq = e.seq
	f.surface = surface
	return f
}

// frameAt is the under-lock half of a frame: the board's rows and the
// debug pane (§21.3.2), which is text already.
func (e *Engine) frameAt(width, height int, now time.Time) frame {
	body := height
	pane, reserved := e.cfg.Scene.Pane(width, height)
	if pane != "" {
		body = max(height-max(reserved, 0), 1)
	}
	return frame{
		snap:       e.cfg.Board.Frame(body, now),
		width:      width,
		height:     body,
		now:        now,
		style:      e.cfg.Style,
		subject:    e.cfg.Subject,
		armedTitle: e.cfg.Board.ArmedTitle(),
		pane:       pane,
	}
}

// render is the lock-free half: the frame's text.
func (f frame) render() string {
	body := LiveRegion(f.snap, f.height, f.width, f.now, f.style)
	if body == "" && f.armedTitle {
		body = ArmedTitleLine(f.subject, f.now, f.style)
	}
	if f.pane == "" {
		return FitLiveRegion(body, f.width)
	}
	return FitLiveRegion(strings.TrimRight(body+f.pane, "\n"), f.width)
}

// paint writes f unless a newer frame or a clear has superseded it, and
// reports whether it wrote. Identical bytes skip WriteLive: force bypasses
// min-gap coalescing, but a spinner tick that reads the same glyph changes
// nothing on screen.
func (e *Engine) paint(f frame, text string) bool {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	if f.seq <= e.floorSeq || f.seq < e.paintedSeq {
		return false
	}
	e.paintedSeq = f.seq
	if text == e.lastText && e.onScreen {
		return false
	}
	start := wall.Now()
	e.lastWrite.Store(&start)
	f.surface.WriteLive(text)
	e.lastText = text
	e.onScreen = true
	e.lastRender.Store(&f.now)
	e.pendingRedraw.Store(false)
	return true
}

// Clear removes the region from the surface and reports whether one was on
// screen. The caller holds the run's lock, so seq is stable: every frame
// snapshotted before this call is dropped, and an unlocked paint still in
// flight can never land after a ClearLive, WriteDurable or WriteFinal.
func (e *Engine) Clear() bool {
	if e == nil {
		return false
	}
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.floorSeq = e.seq
	if !e.onScreen {
		return false
	}
	e.cfg.Surface.ClearLive()
	e.onScreen = false
	e.lastText = ""
	return true
}

// paintLocked snapshots, renders and writes one frame under the run's
// lock: for a caller that needs the frame on screen when it returns.
func (e *Engine) paintLocked() {
	start := wall.Now()
	f := e.snapshot()
	e.paint(f, f.render())
	e.meter.observe(wall.Since(start))
}

// paintUnlocked snapshots under the run's lock and renders and writes with
// it released, so a slow frame never stalls the workers. Enter and leave
// with the lock held.
func (e *Engine) paintUnlocked() {
	start := wall.Now()
	f := e.snapshot()
	e.cfg.Lock.Unlock()
	e.paint(f, f.render())
	e.meter.observe(wall.Since(start))
	e.cfg.Lock.Lock()
}

// shouldSkipGlyph applies skipGlyphRepaint to the last frame and the last
// write. A surface never written to is never skipped.
func (e *Engine) shouldSkipGlyph() bool {
	written := e.lastWrite.Load()
	if written == nil {
		return false
	}
	return skipGlyphRepaint(e.meter.cost(), wall.Since(*written))
}
