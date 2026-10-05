package engine

import (
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/render"
	"github.com/zachbornheimer/evident-output/internal/render/live"
)

// liveFrame is a live region as o.mu saw it: everything render needs, owned
// by the frame. Its Snapshot is built from clones (see taskState.snapshot),
// so rendering it after o.mu is released cannot race a mutating Task.
type liveFrame struct {
	surface       LiveSurface
	seq           uint64
	snap          Snapshot
	width, height int
	now           time.Time
	style         render.Style
	subject       string
	armedTitle    bool
	pane          string
}

// liveFrameLocked snapshots the live region for surface and stamps it with
// the next paint sequence number.
func (o *Output) liveFrameLocked(surface LiveSurface) liveFrame {
	// Refresh geometry before layout when the driver supports it (ANSI + real TTY).
	if r, ok := surface.(interface{ RefreshSize() }); ok {
		r.RefreshSize()
	}
	cols, rows := surface.Columns(), surface.Rows()
	// Keep config width in sync for plain residual paths that still use cfg.text.
	if cols > 0 {
		o.cfg.width = cols
	}
	f := o.liveFrameAtLocked(cols, rows, o.cfg.clock.Now())
	o.live.seq++
	f.seq = o.live.seq
	f.surface = surface
	return f
}

// liveFrameAtLocked is the under-lock half of a live frame: the snapshot
// and the debug pane (§21.3.2), which is text already.
func (o *Output) liveFrameAtLocked(width, height int, now time.Time) liveFrame {
	style := o.humanStyle()
	bodyHeight := height
	var pane strings.Builder
	if o.cfg.debugPresentation == DebugPresentationPane && len(o.debugRecords) > 0 {
		// Reserve rows for pane heading + visible records before budgeting the body.
		paneRows := min(debugPaneReservedRows(o.cfg.debugPane, len(o.debugRecords)), height-1)
		bodyHeight = max(height-max(paneRows, 0), 1)
		writeDebugPane(&pane, o.debugRecords, o.cfg.debugPane, width, style.Color)
	}
	return liveFrame{
		snap:       o.liveSnapshotLocked(bodyHeight, now),
		width:      width,
		height:     bodyHeight,
		now:        now,
		style:      style,
		subject:    o.cfg.subject,
		armedTitle: o.armedTitleLiveLocked(),
		pane:       pane.String(),
	}
}

// render is the lock-free half: the frame's text.
func (f liveFrame) render() string {
	body := live.LiveRegion(f.snap, f.height, f.width, f.now, f.style)
	if body == "" && f.armedTitle {
		body = live.ArmedTitleLine(f.subject, f.now, f.style)
	}
	if f.pane == "" {
		return live.FitLiveRegion(body, f.width)
	}
	return live.FitLiveRegion(strings.TrimRight(body+f.pane, "\n"), f.width)
}

// paint writes f unless a newer frame or a clear has superseded it, and
// reports whether it wrote. Identical bytes skip WriteLive: force bypasses
// min-gap coalescing, but a spinner tick that reads the same glyph changes
// nothing on screen.
func (l *liveEngine) paint(f liveFrame, text string) bool {
	l.paintMu.Lock()
	defer l.paintMu.Unlock()
	if f.seq <= l.floorSeq || f.seq < l.paintedSeq {
		return false
	}
	l.paintedSeq = f.seq
	if text == l.lastLiveText && l.liveActive {
		return false
	}
	start := time.Now()
	l.lastWrite.Store(&start)
	f.surface.WriteLive(text)
	l.lastLiveText = text
	l.liveActive = true
	l.lastRender.Store(&f.now)
	l.pendingRedraw.Store(false)
	return true
}

// clear removes the live region from surface and reports whether one was on
// screen. The caller holds o.mu, so seq is stable: every frame snapshotted
// before this call is dropped, and an unlocked paint still in flight can
// never land after a ClearLive, WriteDurable or WriteFinal.
func (l *liveEngine) clear(surface LiveSurface) bool {
	l.paintMu.Lock()
	defer l.paintMu.Unlock()
	l.floorSeq = l.seq
	if !l.liveActive {
		return false
	}
	surface.ClearLive()
	l.liveActive = false
	l.lastLiveText = ""
	return true
}

// paintFrameLocked snapshots, renders and writes one frame under o.mu: a
// caller that needs the frame on screen when it returns.
func (o *Output) paintFrameLocked(surface LiveSurface) {
	start := time.Now()
	f := o.liveFrameLocked(surface)
	o.live.paint(f, f.render())
	o.live.meter.observe(time.Since(start))
}

// paintFrameUnlocked snapshots under o.mu and renders and writes with it
// released, so a slow frame never stalls the workers. Enter and leave with
// o.mu held.
func (o *Output) paintFrameUnlocked(surface LiveSurface) {
	engine := o.live
	start := time.Now()
	f := o.liveFrameLocked(surface)
	o.mu.Unlock()
	engine.paint(f, f.render())
	engine.meter.observe(time.Since(start))
	o.mu.Lock()
}
