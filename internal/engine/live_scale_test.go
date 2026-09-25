package engine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// liveGroupFrames runs n no-op Tasks under one Group on an interactive
// surface whose clock never advances, and returns the frames painted. With
// a frozen clock every transition lands inside one frame interval, so the
// frame-rate cap is the only thing between n Tasks and O(n) full renders.
func liveGroupFrames(tb testing.TB, n int) int {
	tb.Helper()
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor(), maxConcurrency(1))
	g := out.Group("items")
	for i := range n {
		g.Task(fmt.Sprintf("item %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := g.Wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	frames := screen.frameCount()
	_ = out.Close()
	return frames
}

// TestLiveFramesDoNotScaleWithTasks guards the interactive scheduling
// path: every Task start and settle forced a full live render of every
// Task under o.mu, about two frames per Task (4000 Tasks: 46.9s). Forced
// paints now spend a bounded render budget per frame interval, so the
// frames painted inside one interval stop growing with the run.
func TestLiveFramesDoNotScaleWithTasks(t *testing.T) {
	const n = 2000
	frames := liveGroupFrames(t, n)
	t.Logf("n=%d frames=%d", n, frames)
	if frames > n/10 {
		t.Errorf("painted %d live frames for %d Tasks inside one frame interval; want the render budget to cap them", frames, n)
	}
}

// liveWallClockRun runs n no-op Tasks under one Group on an interactive
// surface with the real clock and the default frame rate, and returns the
// wall time the run took and the frames it painted.
func liveWallClockRun(tb testing.TB, n int) (time.Duration, int) {
	tb.Helper()
	screen := &countingLiveSurface{}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withNoColor(), maxConcurrency(4))
	g := out.Group("items")
	start := time.Now()
	for i := range n {
		g.Task(fmt.Sprintf("item %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := g.Wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	elapsed := time.Since(start)
	frames := screen.frameCount()
	_ = out.Close()
	return elapsed, frames
}

// liveWallClockCeiling is how long 16000 no-op Tasks may take on a live
// surface. The scheduler alone needs well under a second; before live
// paints were metered by their own render cost, every frame re-rendered
// all 16000 rows under o.mu (about 143ms each) and the run took 9m36s.
const liveWallClockCeiling = 30 * time.Second

// TestLiveWallClockDoesNotCollapseAtScale guards E-030 at the size where
// one full render outlasts the frame interval: paints must yield the lock
// in proportion to what they cost, or every signal forces another full
// render and the run spends nearly all of its time drawing.
func TestLiveWallClockDoesNotCollapseAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("wall-clock scale guard")
	}
	const n = 16000
	elapsed, frames := liveWallClockRun(t, n)
	t.Logf("n=%d elapsed=%s frames=%d", n, elapsed, frames)
	if elapsed > liveWallClockCeiling {
		t.Errorf("%d Tasks on a live surface took %s (%d frames); want under %s", n, elapsed, frames, liveWallClockCeiling)
	}
}
