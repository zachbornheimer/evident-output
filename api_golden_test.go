package evo_test

import (
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/vocabulary"
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

// TestAPIGolden_VocabularyFreeze is ZYS-1187: every exported identifier in
// testdata/api_golden.txt (and the live Walk) must appear in
// testdata/api_vocabulary.txt as canonical or helper with a concept. A
// removed classification still present on the surface fails.
func TestAPIGolden_VocabularyFreeze(t *testing.T) {
	entries, err := vocabulary.LoadVocabulary("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("testdata/api_vocabulary.txt is empty")
	}
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	if report := vocabulary.CheckVocabulary(live, entries); !report.OK() {
		t.Fatalf("live surface failed testdata/api_vocabulary.txt freeze:\n%s", report)
	}
	goldenRaw, err := os.ReadFile(apisurface.GoldenRelPath)
	if err != nil {
		t.Fatal(err)
	}
	golden := strings.Split(strings.TrimRight(string(goldenRaw), "\n"), "\n")
	if report := vocabulary.CheckVocabulary(golden, entries); !report.OK() {
		t.Fatalf("testdata/api_golden.txt failed testdata/api_vocabulary.txt freeze:\n%s", report)
	}
}
