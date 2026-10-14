package evo

import (
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestInit_ArmsFirstPaintBeforeAnyEntity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Name: "demo", Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{color: colorNever, visibilityDelay: DelayForTest(0)})
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
	out := initRun(Config{Name: "zq", DryRun: true, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{subtitle: "zq purge", facts: []factRecord{{Name: "scan roots", Value: "~/Developer"}}, color: colorNever, visibilityDelay: DelayForTest(0)})
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
