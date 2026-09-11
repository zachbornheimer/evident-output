package evo_test

import (
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

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
	if !strings.Contains(got, "[") || !strings.Contains(got, "█") || !strings.Contains(got, "░") {
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

// TestLive_DeterminateProgressPhaseIsDefaultIntensity is red-first against
// evo-rec.md's DURING shape `:. name  N/M  muted-current`: the N/M count is
// the diagnostic; the current-name/Phase slot is subordinate, so it renders
// muted (txt.Dim) while 3/10 stays default intensity.
func TestLive_DeterminateProgressPhaseIsDefaultIntensity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24))
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorAlways})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("scan")
	task.Progress(3, 10)
	task.Doing("reading manifest")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "\x1b[2mreading manifest") {
		t.Fatalf("phase must be dimmed while the task is running:\n%q", got)
	}
	if !strings.Contains(got, "reading manifest") {
		t.Fatalf("expected phase text present:\n%q", got)
	}
	if !strings.Contains(got, "3/10") {
		t.Fatalf("expected unit count:\n%s", got)
	}
	if strings.Contains(got, "\x1b[2m3/10") {
		t.Fatalf("count must stay undimmed:\n%q", got)
	}
}
