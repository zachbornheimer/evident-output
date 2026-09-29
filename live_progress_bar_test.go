package evo_test

import (
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestLive_DeterminateProgressBarAndIndeterminatePhase asserts the bar's
// filled cells; it no longer asserts a "░" shaded empty cell, per spec §23
// ("Empty cells are literal spaces, never shaded/outline glyphs") — fixed
// in progressBar (internal/render/live.go) alongside this test.
func TestLive_DeterminateProgressBarAndIndeterminatePhase(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("work")
	units := g.Task("scan")
	bytes := g.Task("fetch")
	spin := g.Task("verify")

	units.Progress(3, 10)
	bytes.Bytes(4_000_000, 10_000_000)
	spin.Doing("checking signatures")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "[") || !strings.Contains(got, "█") {
		t.Fatalf("expected progress bar glyphs:\n%s", got)
	}
	if !strings.Contains(got, "3/10") {
		t.Fatalf("expected unit progress:\n%s", got)
	}
	if !strings.Contains(got, "checking signatures") {
		t.Fatalf("expected indeterminate phase:\n%s", got)
	}
	// Declaration order preserved.
	if strings.Index(got, "scan") > strings.Index(got, "fetch") {
		t.Fatalf("order:\n%s", got)
	}
}

// TestLive_DeterminateProgressPhaseIsFullIntensityActivityChild is red-first
// against spec §17's intensity table ("Current activity: full") and §18/§23
// ("stable parent line with bar/count/timer, one activity child"): a
// standalone Running task with a determinate bar/count and a current-
// activity Phase renders the activity as its own full-intensity child line
// beneath the parent, not muted text fused onto the parent's own line — the
// N/M count is diagnostic, but "reading manifest" is what the task is
// actually doing right now, which spec §17 ranks as full intensity like a
// Task name, never subordinate.
func TestLive_DeterminateProgressPhaseIsFullIntensityActivityChild(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24))
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorAlways})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("scan")
	task.Progress(3, 10)
	task.Doing("reading manifest")

	got := screen.LatestLiveText()
	if strings.Contains(got, "\x1b[2mreading manifest") {
		t.Fatalf("current activity must render at full intensity, not dimmed:\n%q", got)
	}
	if !strings.Contains(got, "reading manifest") {
		t.Fatalf("expected phase text present:\n%q", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "3/10") || strings.Contains(lines[0], "reading manifest") {
		t.Fatalf("expected the parent line to carry only the count, and the activity on its own child line:\n%q", got)
	}
	if strings.Contains(got, "\x1b[2m3/10") {
		t.Fatalf("count must stay undimmed:\n%q", got)
	}
}
