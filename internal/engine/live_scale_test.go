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
