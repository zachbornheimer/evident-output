package engine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// liveFrameTasks is how many Task snapshots a live frame of rows rows
// holds for a run of n Tasks under one Group, half of them finished and
// the rest still pending.
func liveFrameTasks(tb testing.TB, n, rows int) int {
	tb.Helper()
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor(), maxConcurrency(1))
	tb.Cleanup(func() { _ = out.Close() })
	g := out.Group("items")
	for i := range n / 2 {
		g.Task(fmt.Sprintf("done %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := g.Wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	for i := range n / 2 {
		g.Task(fmt.Sprintf("queued %d", i))
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	s := out.liveSnapshotLocked(rows, clock.t)
	kept := len(s.Tasks)
	for _, col := range s.Collections {
		kept += len(col.Tasks)
	}
	return kept
}

// TestLiveFrameSnapshotsOnlyWhatTheScreenShows guards §14 at scale (E-091):
// a frame that snapshotted and rendered every Task grew with the run, and
// the paint cooldown that hid its cost left the frame unchanged for 0.9s
// at 16000 Tasks. A frame now snapshots at most the children its rows
// could show, so its work does not grow with n.
func TestLiveFrameSnapshotsOnlyWhatTheScreenShows(t *testing.T) {
	const rows = 24
	small, large := liveFrameTasks(t, 1000, rows), liveFrameTasks(t, 16000, rows)
	t.Logf("rows=%d kept: n=1000 %d, n=16000 %d", rows, small, large)
	if large != small {
		t.Errorf("a %d-row frame snapshotted %d Tasks at n=16000 and %d at n=1000; want the same bound whatever the run size", rows, large, small)
	}
	if large > 10*rows {
		t.Errorf("a %d-row frame snapshotted %d Tasks; want a bound near the screen's rows", rows, large)
	}
}
