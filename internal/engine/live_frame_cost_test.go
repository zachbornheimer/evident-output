package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/scaletest"
)

// liveFrameOutput is an interactive Output over n Tasks under one Group,
// half finished and half queued: the state a spinner tick repaints.
func liveFrameOutput(tb testing.TB, n int) (*Output, time.Time) {
	tb.Helper()
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(&countingLiveSurface{}), visibilityDelay(0), withClock(clock), withNoColor(), maxConcurrency(1))
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
	return out, clock.t
}

// liveFrameFull is one spinner-tick frame: the snapshot under o.mu and the
// text render after it, as the animator builds it.
func liveFrameFull(out *Output, now time.Time) string {
	return out.renderLiveRegionWithDebugLocked(80, 24, now)
}

// renderLiveRegionWithDebugLocked builds one frame the way the animator does.
// o.mu is held.
func (o *Output) renderLiveRegionWithDebugLocked(width, height int, now time.Time) string {
	return o.liveFrameAtLocked(width, height, now).render()
}

// liveFrameCost is the fastest of several repaints of an n-Task Group, so
// a scheduling hiccup on a loaded host does not read as growth.
func liveFrameCost(tb testing.TB, n int) time.Duration {
	tb.Helper()
	out, now := liveFrameOutput(tb, n)
	out.mu.Lock()
	defer out.mu.Unlock()
	_ = liveFrameFull(out, now)
	return scaletest.Fastest(scaletest.CheapSamples*2, func() time.Duration {
		return scaletest.Elapsed(func() { _ = liveFrameFull(out, now) })
	})
}

// liveFrameTimerFloor is the noise one timed frame carries on a loaded
// host: a frame is tens of microseconds, so a single timer tick is a large
// share of it.
const liveFrameTimerFloor = 100 * time.Microsecond

// TestLiveFrameCostDoesNotGrowWithOffScreenTasks guards E-091 in time: a
// repaint that walked every Task would cost 16x more at 16000 than at 1000.
func TestLiveFrameCostDoesNotGrowWithOffScreenTasks(t *testing.T) {
	small, large := liveFrameCost(t, 1000), liveFrameCost(t, 16000)
	t.Logf("per-frame: n=1000 %s, n=16000 %s", small, large)
	if large > 2*small+liveFrameTimerFloor {
		t.Errorf("a frame took %s at 16000 Tasks and %s at 1000; want within 2x", large, small)
	}
}
