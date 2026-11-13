package evo_test

import (
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestVisibilityDelay_WithholdsLiveUntilElapsed(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(150 * time.Millisecond), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	if got := screen.LiveFrameCount(); got != 0 {
		t.Fatal("pending declare must be withheld until VisibilityDelay elapses")
	}

	clock.Advance(150 * time.Millisecond)
	task.Doing("fetching")
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("expected live frame after VisibilityDelay elapsed")
	}
	live := screen.LatestLiveText()
	if live == "" || !hasSpinnerGlyph(live) {
		t.Fatalf("submitted work must spin after delay elapsed, live=%q", live)
	}

	task.Done()
	_ = out.Finish()
}

func TestVisibilityDelay_ZeroIsImmediate(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("work")
	task.Doing("running")
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("VisibilityDelay(0) must paint immediately")
	}
	task.Done()
	_ = out.Finish()
}

func TestTask_AlonePaintsLiveWithZeroDelay(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
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
