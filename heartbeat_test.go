package evo_test

import (
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// A sibling task's Progress call forces a fresh live render at the current
// domain-clock time without touching the task under test's own activityAt —
// the same technique TestLive_SpinnerGlyphAdvancesWithClock uses.
func forceRender(ticker *evo.TaskHandle, n int) {
	ticker.Progress(n, 100)
}

func TestHeartbeat_AppearsAfterPhaseGoesStale(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	push := out.Task("push")
	ticker := out.Task("ticker")

	push.Phase("pushing feat/a")
	forceRender(ticker, 1)
	if strings.Contains(screen.LatestLiveText(), "—") {
		t.Fatalf("heartbeat appeared before staleness:\n%s", screen.LatestLiveText())
	}

	clock.Advance(11 * time.Second)
	forceRender(ticker, 2)
	live := screen.LatestLiveText()
	if !strings.Contains(live, "pushing feat/a — 11s") {
		t.Fatalf("expected heartbeat suffix after 11s stale:\n%s", live)
	}

	push.Done()
	ticker.Done()
	_ = out.Finish()
}

func TestHeartbeat_ResetsOnPhaseUpdate(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	push := out.Task("push")
	ticker := out.Task("ticker")

	push.Phase("pushing feat/a")
	clock.Advance(11 * time.Second)
	forceRender(ticker, 1)
	if !strings.Contains(screen.LatestLiveText(), "— 11s") {
		t.Fatal("expected heartbeat before reset")
	}

	push.Phase("pushing feat/b") // fresh phase resets the elapsed clock
	forceRender(ticker, 2)
	if strings.Contains(screen.LatestLiveText(), "—") {
		t.Fatalf("heartbeat must reset on Phase update:\n%s", screen.LatestLiveText())
	}

	push.Done()
	ticker.Done()
	_ = out.Finish()
}

func TestHeartbeat_AbsentWhileProgressAdvances(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install")
	install.Phase("installing")

	for i := 1; i <= 3; i++ {
		clock.Advance(11 * time.Second) // each step alone would be stale
		install.Progress(i, 3)          // but Progress keeps resetting activity
	}
	if strings.Contains(screen.LatestLiveText(), "—") {
		t.Fatalf("heartbeat must stay absent while progress advances:\n%s", screen.LatestLiveText())
	}

	install.Done()
	_ = out.Finish()
}

func TestHeartbeat_AbsentInPlainProjection(t *testing.T) {
	var buf strings.Builder
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.Clock(clock)}})

	push := out.Task("push")
	push.Phase("pushing feat/a")
	clock.Advance(90 * time.Second)
	push.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "—") {
		t.Fatalf("plain projection must never show a heartbeat suffix:\n%s", buf.String())
	}
}
