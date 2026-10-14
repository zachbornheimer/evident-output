package evo

import (
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/testkit"
)

func TestInit_ArmsFirstPaintBeforeAnyEntity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{
		stdout: io.Discard, stderr: io.Discard,
		title:           "demo",
		terminal:        screen,
		color:           colorNever,
		visibilityDelay: DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.close() })

	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Init must paint a live frame before any entity is declared")
	}
	if !strings.Contains(screen.LatestLiveText(), "demo") {
		t.Fatalf("armed title line missing subject, got %q", screen.LatestLiveText())
	}
}

// Isolated means "not the package default", not "skip first paint". Nested
// Isolated inits (zq purge/prune preview) used to emit [dry-run] and then
// freeze while Inventory ran — Isolated must still arm.
func TestInit_IsolatedDryRunArmsAndEmitsConstructionFacts(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{
		title:           "zq",
		subject:         "zq purge",
		DryRun:          true,
		facts:           []factRecord{{Name: "scan roots", Value: "~/Developer"}},
		stdout:          io.Discard,
		stderr:          io.Discard,
		terminal:        screen,
		color:           colorNever,
		visibilityDelay: DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.close() })

	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Isolated Init must paint a live frame before any Task; Isolated is not a first-paint exemption")
	}
	if live := screen.LatestLiveText(); !strings.Contains(live, "zq") {
		t.Fatalf("armed title missing subject, live=%q", live)
	}
	snap := out.snapshot()
	if !snap.DryRun {
		t.Fatal("DryRun must be on the snapshot")
	}
	if len(snap.Facts) != 1 || snap.Facts[0].Name != "scan roots" || snap.Facts[0].Value != "~/Developer" {
		t.Fatalf("construction Facts missing from snapshot: %+v", snap.Facts)
	}
	persisted := screen.PersistedText()
	if !strings.Contains(persisted, "[dry-run]") {
		t.Fatalf("dry-run header missing from persisted text:\n%s", persisted)
	}
	if !strings.Contains(persisted, "scan roots") || !strings.Contains(persisted, "~/Developer") {
		t.Fatalf("construction Facts must persist with the header:\n%s", persisted)
	}
}
