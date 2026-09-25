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

// liveFrameNestedTasks is how many Task snapshots a live frame of rows
// rows holds for a Group of n per-item Groups of two Tasks each, the
// per-item shape E-091's flat bound missed.
func liveFrameNestedTasks(tb testing.TB, n, rows int) int {
	tb.Helper()
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor(), maxConcurrency(1))
	tb.Cleanup(func() { _ = out.Close() })
	items := out.Group("items")
	for i := range n {
		item := items.Group(fmt.Sprintf("item %d", i))
		item.Task("check")
		item.Task("apply")
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	return countSnapshotTasks(out.liveSnapshotLocked(rows, clock.t).Collections)
}

func countSnapshotTasks(cols []TasksSnapshot) int {
	n := 0
	for _, col := range cols {
		n += len(col.Tasks) + countSnapshotTasks(col.Collections)
	}
	return n
}

// TestLiveFrameBoundHoldsForNestedCollections pins E-091/E-096's
// re-verify: the frame bound held only for flat children, and a Group of
// per-item Groups snapshotted every nested Task (32000 at n=16000, 651ms
// a frame under o.mu). Nested collections now project through the same
// row budget, so the kept count does not grow with n.
func TestLiveFrameBoundHoldsForNestedCollections(t *testing.T) {
	const rows = 24
	small, large := liveFrameNestedTasks(t, 1000, rows), liveFrameNestedTasks(t, 16000, rows)
	t.Logf("rows=%d kept: n=1000 %d, n=16000 %d", rows, small, large)
	if large != small {
		t.Errorf("a %d-row frame snapshotted %d nested Tasks at n=16000 and %d at n=1000; want the same bound", rows, large, small)
	}
	if large > 10*rows {
		t.Errorf("a %d-row frame snapshotted %d nested Tasks; want a bound near the screen's rows", rows, large)
	}
}

// nestedFrameAllocs is how many allocations building one live frame of
// rows rows costs for a Group of n per-item Groups, half finished.
func nestedFrameAllocs(tb testing.TB, n, rows int) float64 {
	tb.Helper()
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor(), maxConcurrency(1))
	tb.Cleanup(func() { _ = out.Close() })
	items := out.Group("items")
	for i := range n / 2 {
		items.Group(fmt.Sprintf("done %d", i)).Task("check").Define(func(context.Context) error { return nil })
	}
	if err := items.Wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	for i := range n / 2 {
		items.Group(fmt.Sprintf("queued %d", i)).Task("check")
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	return testing.AllocsPerRun(3, func() { _ = out.liveSnapshotLocked(rows, clock.t) })
}

// TestLiveFrameCostHoldsForNestedCollections pins E-091/E-096's round-7
// re-verify: a frame kept only the reachable per-item Groups but still
// built a view of every other one (and walked its Tasks) to tally it, on
// every settle under o.mu, so a Group of per-item Groups ran O(n²) under a
// live terminal (n=16000: one 5m57s frame gap). A frame now tallies an
// unreachable collection from its census, so its cost does not grow with n.
func TestLiveFrameCostHoldsForNestedCollections(t *testing.T) {
	const rows = 24
	small, large := nestedFrameAllocs(t, 1000, rows), nestedFrameAllocs(t, 16000, rows)
	t.Logf("rows=%d allocs/frame: n=1000 %.0f, n=16000 %.0f", rows, small, large)
	if large > small {
		t.Errorf("a %d-row frame allocated %.0f times at n=16000 and %.0f at n=1000; want the same bound whatever the run size", rows, large, small)
	}
}
