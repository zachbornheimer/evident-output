package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestLive_DeterminateProgressBarAndIndeterminatePhase(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	g := out.Tasks("work")
	units := g.Task("scan")
	bytes := g.Task("fetch")
	spin := g.Task("verify")

	units.Progress(3, 10)
	bytes.Bytes(4_000_000, 10_000_000)
	spin.Phase("checking signatures")

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

func TestLive_DeterminateProgressPhaseIsDimmed(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24))
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("scan")
	task.Progress(3, 10)
	task.Phase("reading manifest")

	got := screen.LatestLiveText()
	dimmedPhase := "\x1b[2mreading manifest\x1b[0m"
	if !strings.Contains(got, dimmedPhase) {
		t.Fatalf("expected dimmed Phase on determinate Progress+Phase:\n%q", got)
	}
	if !strings.Contains(got, "3/10") {
		t.Fatalf("expected unit count:\n%s", got)
	}
	if strings.Contains(got, "\x1b[2m3/10") {
		t.Fatalf("count must stay undimmed:\n%q", got)
	}
}
