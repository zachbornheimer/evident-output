package adopt_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/adopt"
)

// TestInventoryMixedFixtureGolden pins adopt.Inventory's output against
// testdata/mixed, a fixture that mixes fmt.Printf, log.Println, log.Fatal,
// and a manual spinner import — the exact shape a real pre-adoption CLI
// carries. Run with -update to regenerate the golden after a deliberate
// detection change.
func TestInventoryMixedFixtureGolden(t *testing.T) {
	got, err := adopt.Inventory(filepath.Join("testdata", "mixed"))
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	// Golden compares on file basename only — the fixture's absolute path
	// depends on where the module checkout lives.
	for i := range got.Findings {
		got.Findings[i].File = filepath.Base(got.Findings[i].File)
	}
	got.Directory = filepath.Base(got.Directory)

	goldenPath := filepath.Join("testdata", "mixed.golden.json")
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotJSON = append(gotJSON, '\n')

	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, gotJSON, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(gotJSON) != string(want) {
		t.Errorf("Inventory(testdata/mixed) drifted from golden.\ngot:\n%s\nwant:\n%s\n(re-run with UPDATE_GOLDEN=1 if this drift is deliberate)", gotJSON, want)
	}
}

// TestInventoryFlagsEveryFixtureSite is the non-golden half: it names each
// site the golden pins so a future refactor of the golden format cannot
// silently drop coverage of one.
func TestInventoryFlagsEveryFixtureSite(t *testing.T) {
	plan, err := adopt.Inventory(filepath.Join("testdata", "mixed"))
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	wantPatterns := map[string]bool{
		"import github.com/briandowns/spinner": false,
		"fmt.Printf":                           false,
		"log.Println":                          false,
		"log.Fatal":                            false,
	}
	for _, f := range plan.Findings {
		if _, ok := wantPatterns[f.Pattern]; ok {
			wantPatterns[f.Pattern] = true
		}
		if f.Suggestion == "" {
			t.Errorf("finding %+v has no suggestion — adopt must always say why or say it can't", f)
		}
	}
	for pattern, found := range wantPatterns {
		if !found {
			t.Errorf("Inventory missed expected pattern %q: %+v", pattern, plan.Findings)
		}
	}
}

// TestInventoryUnknownDirectory proves Inventory reports an honest error
// rather than an empty, silently-successful plan.
func TestInventoryUnknownDirectory(t *testing.T) {
	if _, err := adopt.Inventory(filepath.Join("testdata", "does-not-exist")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}
