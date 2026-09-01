package evo_test

import (
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestVisibilityDelay_WithholdsLiveUntilElapsed(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.NewWithOptions(
		evo.Terminal(screen),
		evo.Clock(clock),
		evo.VisibilityDelay(150*time.Millisecond),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	task.Phase("fetching") // activity — should wait for delay
	if got := screen.LiveFrameCount(); got != 0 {
		t.Fatalf("live frames before delay = %d, want 0", got)
	}

	clock.Advance(50 * time.Millisecond)
	task.Phase("still fetching") // still within delay
	if got := screen.LiveFrameCount(); got != 0 {
		t.Fatalf("live frames mid-delay = %d, want 0", got)
	}

	clock.Advance(120 * time.Millisecond) // total 170ms >= 150ms
	task.Progress(1, 2)                   // re-enter signalLive past delay
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("expected live frame after VisibilityDelay elapsed")
	}

	task.Done()
	_ = out.Finish()
}

func TestVisibilityDelay_ZeroIsImmediate(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.NewWithOptions(
		evo.Terminal(screen),
		evo.VisibilityDelay(0),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("work")
	task.Phase("running")
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("VisibilityDelay(0) must paint immediately")
	}
	task.Done()
	_ = out.Finish()
}

func TestTask_AlonePaintsLiveWithZeroDelay(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.NewWithOptions(
		evo.Terminal(screen),
		evo.VisibilityDelay(0),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("work")
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Task() with VisibilityDelay(0) must paint a live frame without Phase")
	}
	_ = out.Finish()
}

func TestDefaultVisibilityDelay_WithinUserSLA(t *testing.T) {
	cfg := evo.DefaultConfig()
	if cfg.VisibilityDelay == nil {
		t.Fatal("DefaultConfig VisibilityDelay must be set")
	}
	const sla = 80 * time.Millisecond
	if *cfg.VisibilityDelay > sla {
		t.Fatalf("default VisibilityDelay = %v, want ≤ %v", *cfg.VisibilityDelay, sla)
	}
}
