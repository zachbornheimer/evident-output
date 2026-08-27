package evo_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/width"
	"github.com/zachbornheimer/evident-output/testkit"
)

// Appendix H interactive cases (v0.2) — written red-first against live terminal.

func TestH2_Task_InstantCompletionDoesNotFlashSpinner(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.NoColor(),
	)
	clock := testkit.NewClock()

	out := evo.NewWithOptions(
		evo.Terminal(screen),
		evo.Clock(clock),
		evo.VisibilityDelay(150*time.Millisecond),
	)
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Task("dependencies")
	dependencies.Donef("installed %d packages", 18)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if got := screen.LiveFrameCount(); got != 0 {
		t.Fatalf("live frames = %d, want 0", got)
	}
	final := screen.FinalText()
	if final == "" {
		t.Fatal("final text empty: interactive Finish must write final projection")
	}
	if !strings.Contains(final, "installed 18 packages") {
		t.Fatalf("final missing completion summary:\n%s", final)
	}
	if strings.ContainsAny(final, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		t.Fatalf("final output contains spinner:\n%s", final)
	}
}

func TestH17_Debug_MessageIsInsertedAboveLiveRegion(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.NoColor(),
	)

	// FixedClock freezes spinner glyphs for stable operation expectations.
	fixed := evo.FixedClock{T: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := evo.NewWithOptions(
		evo.Terminal(screen), evo.VisibilityDelay(0),
		evo.DebugLevel(evo.Debug),
		evo.NoColor(), // assert exact final text without SGR
		evo.Clock(fixed),
	)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("dependencies")
	task.Phase("resolving packages")
	out.Debug("package index loaded", evo.Int("packages", 18))
	task.Donef("installed %d packages", 18)
	_ = out.Finish()

	// History mode: timestamp (FixedClock) + bracketed level above live region.
	want := []testkit.Operation{
		testkit.DrawLive("⠋  dependencies  resolving packages"),
		testkit.ClearLive(),
		testkit.WriteDurable("00:00:00.000 [DEBUG] package index loaded  packages=18"),
		testkit.DrawLive("⠋  dependencies  resolving packages"),
		testkit.ClearLive(),
		testkit.WriteFinal("✓  dependencies  installed 18 packages"),
	}

	if diff := testkit.DiffOperations(want, screen.Operations()); diff != "" {
		t.Fatalf("terminal operations differ (-want +got):\n%s\ngot=%#v", diff, screen.Operations())
	}
}

func TestH20_Tasks_MultipleProgressRowsPreserveDeclarationOrder(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.NoColor(),
	)

	fixed := evo.FixedClock{T: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(fixed), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Tasks("dependencies")
	react := dependencies.Task("react")
	esbuild := dependencies.Task("esbuild")
	sharp := dependencies.Task("sharp")

	// Update and resolve in a different order than declaration.
	sharp.Phase("verifying")
	esbuild.Bytes(12_400_000, 18_000_000)
	react.Bytes(8_100_000, 8_100_000)
	react.Done()

	got := screen.LatestLiveText()
	// Column layout: child names pad to width 9 (spec H.20 semantics: declaration
	// order + absolute bytes + phases). Spacing normalized to a single rule.
	// Bytes rows include an ASCII bar plus fixed MB fraction (live progress UX).
	want := `⠋  dependencies  1/3 complete
   ✓  react      8.1 MB
   ⠋  esbuild    [████████░░░░]  12.4/18.0 MB
   ⠋  sharp      verifying`

	if got != want {
		t.Fatalf("live output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	// Declaration order preserved even when updates arrive out of order.
	reactAt := strings.Index(got, "react")
	esbuildAt := strings.Index(got, "esbuild")
	sharpAt := strings.Index(got, "sharp")
	if reactAt >= esbuildAt || esbuildAt >= sharpAt {
		t.Fatalf("declaration order broken: react=%d esbuild=%d sharp=%d", reactAt, esbuildAt, sharpAt)
	}
}

func TestH21_Tasks_ScreenBudgetSelectsImportantRowsAndReportsOmission(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.Height(8),
		testkit.NoColor(),
	)

	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0))
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Tasks("dependencies")
	for n := 0; n < 120; n++ {
		task := dependencies.Task(fmt.Sprintf("package-%03d", n))
		switch n {
		case 7:
			task.Fail("checksum mismatch")
		case 12, 18:
			task.Phase("downloading")
		case 20:
			task.Warn("using cached fallback")
		default:
			task.Done()
		}
	}

	got := screen.LatestLiveText()
	for _, required := range []string{
		"package-007",
		"checksum mismatch",
		"package-020",
		"package-012",
		"not shown",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("live output omitted %q:\n%s", required, got)
		}
	}
}

func TestH22_Task_HighFrequencyProgressIsCoalesced(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.NoColor(),
	)
	clock := testkit.NewClock()

	out := evo.NewWithOptions(
		evo.Terminal(screen), evo.VisibilityDelay(0),
		evo.Clock(clock),
		evo.MaxFrameRate(30),
	)
	t.Cleanup(func() { _ = out.Close() })

	download := out.Task("download")
	for completed := int64(0); completed <= 10_000; completed++ {
		download.Progress64(completed, 10_000)
		// Keep wall-clock zero; coalescing uses frame budget, not only time.
	}
	download.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if got := download.Snapshot().Progress.Completed; got != 10_000 {
		t.Fatalf("completed = %d, want 10000", got)
	}
	if frames := screen.LiveFrameCount(); frames >= 10_000 {
		t.Fatalf("frames = %d, progress updates were not coalesced", frames)
	}
	if frames := screen.LiveFrameCount(); frames < 1 {
		t.Fatal("expected at least one live frame during progress")
	}
}

func TestLive_RepeatedStyledPhasesFitTerminalWidth(t *testing.T) {
	const columns = 40
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(columns),
	)
	out := evo.NewWithOptions(
		evo.Terminal(screen),
		evo.VisibilityDelay(0),
		evo.Clock(evo.FixedClock{T: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}),
	)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("goimports check")
	task.Phase("goimports -d " + strings.Repeat("file.go ", 20))
	task.Phase("goimports -d " + strings.Repeat("1️⃣ ", 20))

	for _, operation := range screen.Operations() {
		if operation.Kind != "live" {
			continue
		}
		for _, line := range strings.Split(operation.Text, "\n") {
			if cells := width.VisibleCells(line); cells > columns {
				t.Fatalf("live line uses %d cells, terminal has %d:\n%s", cells, columns, operation.Text)
			}
		}
	}
	if frames := screen.LiveFrameCount(); frames != 2 {
		t.Fatalf("live frames=%d, want 2", frames)
	}
	if got := width.StripANSI(screen.LatestLiveText()); !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated live line must signal omitted text:\n%s", got)
	}
}
