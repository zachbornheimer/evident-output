package evo_test

import (
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

// api_golden_test.go is §46's public API golden: the live exported surface
// of the root evo package must equal testdata/api_golden.txt, contain every
// identifier in testdata/api_required.txt, and contain none of
// apisurface.RetiredNames. Regenerating the golden cannot drop a required
// name or revive a retired one. `evident-output contract` and
// `mise run api-contract` run the same check.

func TestAPIGolden_PublicSurfaceMatchesCommittedGolden(t *testing.T) {
	if os.Getenv("UPDATE_API_GOLDEN") == "1" {
		live, err := apisurface.Walk(".")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(apisurface.GoldenRelPath, []byte(strings.Join(live, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", apisurface.GoldenRelPath, err)
		}
	}

	report, err := apisurface.CheckDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK() {
		t.Fatalf("public API surface failed contract (regenerate testdata/api_golden.txt deliberately if this is an intended change):\n%s", report)
	}
}
