package evo

import (
	"io"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestVisibilityDelay_WithholdsLiveUntilElapsed(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: clock, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(150 * time.Millisecond), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	armed := screen.LiveFrameCount()
	if armed == 0 {
		t.Fatal("Isolated Init must still first-paint; VisibilityDelay does not hide the armed title")
	}

	task := out.Task("download")
	if got := screen.LiveFrameCount(); got != armed {
		t.Fatal("pending declare must not add a live frame until VisibilityDelay elapses")
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

	succeed(task)
	_ = out.finish()
}

func TestVisibilityDelay_ZeroIsImmediate(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("work")
	task.Doing("running")
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("VisibilityDelay(0) must paint immediately")
	}
	succeed(task)
	_ = out.finish()
}

func TestTask_AlonePaintsLiveWithZeroDelay(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.Task("work")
	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Task() with VisibilityDelay(0) must paint a live frame without Phase")
	}
	_ = out.finish()
}

func TestDefaultVisibilityDelay_WithinUserSLA(t *testing.T) {
	got := resolveConfig(Config{Output: Plain}, tuning{}).visibilityDelay
	const sla = 80 * time.Millisecond
	if got == 0 || got > sla {
		t.Fatalf("default visibility delay = %v, want in (0, %v]", got, sla)
	}
}
