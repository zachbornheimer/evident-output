package evo_test

import (
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// A sibling task's Progress call forces a fresh live render at the current
// domain-clock time without touching the task under test's own liveFirstSeen
// anchor — the same technique TestLive_SpinnerGlyphAdvancesWithClock uses.
func forceRender(ticker *evo.TaskHandle, n int) {
	ticker.Progress(n, 100)
}

// TestHeartbeat_AppearsAfterElapsedThreshold is the red-first case for P5's
// single monotonic elapsed-time mechanism: a row gains " — Ns" once
// elapsedAfter (5s) has passed since it was first actually painted in the
// live region.
func TestHeartbeat_AppearsAfterElapsedThreshold(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	push := out.Task("push")
	ticker := out.Task("ticker")

	push.Doing("pushing feat/a")
	forceRender(ticker, 1) // first live render: anchors push's elapsed clock
	if strings.Contains(screen.LatestLiveText(), "—") {
		t.Fatalf("elapsed suffix appeared before the 5s threshold:\n%s", screen.LatestLiveText())
	}

	clock.Advance(5 * time.Second)
	forceRender(ticker, 2)
	live := screen.LatestLiveText()
	if !strings.Contains(live, "pushing feat/a — 5s") {
		t.Fatalf("expected elapsed suffix after 5s:\n%s", live)
	}

	push.Done()
	ticker.Done()
	_ = out.Finish()
}

// TestHeartbeat_NeverResetsOnPhaseUpdate is the red-first case for P5's
// "monotonic, NEVER resets on Phase/Progress activity" — a fresh Phase call
// must not restart the elapsed clock (the old phaseStaleAfter heartbeat did;
// this is a deliberate behavior change, not test-weakening).
func TestHeartbeat_NeverResetsOnPhaseUpdate(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	push := out.Task("push")
	ticker := out.Task("ticker")

	push.Doing("pushing feat/a")
	forceRender(ticker, 1) // anchors push's elapsed clock
	clock.Advance(5 * time.Second)
	forceRender(ticker, 2)
	if !strings.Contains(screen.LatestLiveText(), "— 5s") {
		t.Fatal("expected elapsed suffix before the Phase update")
	}

	push.Doing("pushing feat/b") // must NOT reset the elapsed clock
	forceRender(ticker, 3)
	if !strings.Contains(screen.LatestLiveText(), "pushing feat/b — 5s") {
		t.Fatalf("elapsed suffix must survive a Phase update unreset:\n%s", screen.LatestLiveText())
	}

	push.Done()
	ticker.Done()
	_ = out.Finish()
}

// TestHeartbeat_AppearsRegardlessOfProgressActivity is the red-first case
// for P5's "never resets on Phase/Progress activity" applied to Progress:
// repeated Progress calls must not hold the elapsed suffix off past
// elapsedAfter (the old activityAt-anchored heartbeat did).
func TestHeartbeat_AppearsRegardlessOfProgressActivity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install")
	install.Progress(0, 3) // first live render: anchors install's elapsed clock

	clock.Advance(5 * time.Second)
	install.Progress(1, 3)
	clock.Advance(5 * time.Second)
	install.Progress(2, 3) // still advancing, but 10s have now passed

	if !strings.Contains(screen.LatestLiveText(), "—") {
		t.Fatalf("elapsed suffix must appear past threshold even while progress advances:\n%s", screen.LatestLiveText())
	}

	install.Done()
	_ = out.Finish()
}

func TestHeartbeat_AbsentInPlainProjection(t *testing.T) {
	var buf strings.Builder
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.Clock(clock)}})

	push := out.Task("push")
	push.Doing("pushing feat/a")
	clock.Advance(90 * time.Second)
	push.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "—") {
		t.Fatalf("plain projection must never show a heartbeat suffix:\n%s", buf.String())
	}
}
