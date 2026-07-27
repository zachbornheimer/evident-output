package evo_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func fixedDebugClock() evo.FixedClock {
	return evo.FixedClock{T: time.Date(2026, 7, 27, 12, 4, 18, 219_000_000, time.UTC)}
}

// History mode (default): durable append-above, compact grammar with timestamp (§21.3.1).
func TestDebugHistory_AppendAboveLiveRegion(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.New(
		evo.Terminal(screen),
		evo.DebugLevel(evo.Debug),
		evo.DebugHistory(),
		evo.NoColor(),
		evo.Clock(fixedDebugClock()),
		evo.VisibilityDelay(0),
	)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	task.Phase("comparing")
	out.Debug("opened repository", evo.String("path", "/work/repo"))
	task.Done()
	_ = out.Finish()

	var durable string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" {
			durable += op.Text
		}
	}
	if !strings.Contains(durable, "12:04:18.219 [DEBUG] opened repository  path=/work/repo") {
		t.Fatalf("history line missing or wrong format:\nops=%#v\ndurable=%q", screen.Operations(), durable)
	}
	// Success: no diagnostic tail section in final.
	if strings.Contains(screen.FinalText(), "── diagnostics") {
		t.Fatalf("history mode must not emit pane diagnostics tail by default:\n%s", screen.FinalText())
	}
}

// Pane mode: debug lives in the live region; newest first; slog text grammar (§21.3.2).
func TestDebugPane_RollingViewportNewestFirst(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	// Advance so successive Debug calls get distinct times if clock ticks.
	out := evo.New(
		evo.Terminal(screen),
		evo.DebugLevel(evo.Debug),
		evo.DebugPane(evo.PaneHeight(2), evo.NewestFirst()),
		evo.NoColor(),
		evo.Clock(clock),
		evo.VisibilityDelay(0),
	)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("work")
	task.Phase("running")
	out.Debug("first event")
	clock.Advance(time.Millisecond)
	out.Debug("second event")
	clock.Advance(time.Millisecond)
	out.Debug("third event") // pane height 2 → first event leaves viewport

	live := screen.LatestLiveText()
	if !strings.Contains(live, "── debug · newest first ──") {
		t.Fatalf("live missing debug pane heading:\n%s", live)
	}
	if !strings.Contains(live, `msg="third event"`) {
		t.Fatalf("newest record missing from pane:\n%s", live)
	}
	if !strings.Contains(live, `msg="second event"`) {
		t.Fatalf("second-newest missing from pane:\n%s", live)
	}
	if strings.Contains(live, `msg="first event"`) {
		t.Fatalf("pane height 2 should drop oldest visible record:\n%s", live)
	}
	// level=DEBUG slog grammar (not bracket history inside pane).
	if strings.Contains(live, "[DEBUG]") {
		t.Fatalf("pane must use slog text, not bracket history:\n%s", live)
	}
	if !strings.Contains(live, "level=DEBUG") {
		t.Fatalf("pane missing level=DEBUG:\n%s", live)
	}

	// No durable history dump while pane owns presentation.
	for _, op := range screen.Operations() {
		if op.Kind == "durable" && strings.Contains(op.Text, "event") {
			t.Fatalf("pane mode must not WriteDurable debug history mid-run: %q", op.Text)
		}
	}

	task.Done()
	_ = out.Finish()
	// Success: pane removed; no diagnostics tail.
	final := screen.FinalText()
	if strings.Contains(final, "── debug") || strings.Contains(final, "── diagnostics") {
		t.Fatalf("success must remove pane and not preserve tail:\n%s", final)
	}
	if strings.Contains(final, "third event") {
		t.Fatalf("success final must not retain pane records:\n%s", final)
	}
}

// Failure preserves a bounded diagnostic tail under the final result (§21.3.2).
func TestDebugPane_FailurePreservesDiagnosticTail(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var primary bytes.Buffer
	out := evo.New(
		evo.To(&primary),
		evo.Terminal(screen),
		evo.DebugLevel(evo.Debug),
		evo.DebugPane(evo.PaneHeight(5), evo.NewestFirst()),
		evo.NoColor(),
		evo.Clock(fixedDebugClock()),
		evo.VisibilityDelay(0),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Phase("running")
	out.Debug("enumerated local branches", evo.Int("count", 7))
	out.Debug("fetched remote metadata", evo.String("remote", "origin"))
	out.Item("disk").Fail("full")
	_ = out.Finish()

	got := primary.String()
	if !strings.Contains(got, "[failed]") {
		t.Fatalf("expected failed conclusion:\n%s", got)
	}
	if !strings.Contains(got, "── diagnostics ──") {
		t.Fatalf("failure must preserve labeled diagnostic tail:\n%s", got)
	}
	if !strings.Contains(got, `msg="fetched remote metadata"`) {
		t.Fatalf("tail missing slog-formatted records:\n%s", got)
	}
	// Newest first in tail.
	idxNew := strings.Index(got, `msg="fetched remote metadata"`)
	idxOld := strings.Index(got, `msg="enumerated local branches"`)
	if idxNew < 0 || idxOld < 0 || idxNew > idxOld {
		t.Fatalf("tail should list newest first:\n%s", got)
	}
}

// PreserveDebugTail forces a tail even on success (explicit opt-in).
func TestDebugPane_PreserveDebugTailAlways(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
		evo.DebugLevel(evo.Debug),
		// Plain cannot show a live pane; history streams, but PreserveDebugTail still
		// requests a diagnostics section at Finish when presentation is pane-configured.
		evo.DebugPane(evo.PreserveDebugTail(), evo.PaneHeight(3)),
		evo.Clock(fixedDebugClock()),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Debug("cache warm", evo.String("dir", "/tmp/x"))
	out.Item("ok").OK()
	_ = out.Finish()
	got := buf.String()
	if !strings.Contains(got, "── diagnostics ──") {
		t.Fatalf("PreserveDebugTail should emit diagnostics section:\n%s", got)
	}
	if !strings.Contains(got, `msg="cache warm"`) {
		t.Fatalf("tail missing record:\n%s", got)
	}
}

// Plain + history: still one stream, no double print (regression).
func TestDebugHistory_PlainStreamsOnce(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
		evo.DebugLevel(evo.Debug),
		evo.DebugHistory(),
		evo.Clock(fixedDebugClock()),
	)
	t.Cleanup(func() { _ = out.Close() })
	out.Debug("cache warm", evo.String("dir", "/tmp/x"))
	if n := strings.Count(buf.String(), "[DEBUG] cache warm"); n != 1 {
		t.Fatalf("before Finish count=%d:\n%s", n, buf.String())
	}
	_ = out.Finish()
	if n := strings.Count(buf.String(), "[DEBUG] cache warm"); n != 1 {
		t.Fatalf("after Finish count=%d:\n%s", n, buf.String())
	}
}
