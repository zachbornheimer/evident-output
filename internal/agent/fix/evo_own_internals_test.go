package fix_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/fix"
)

// TestAnalyzersLeaveEvoOwnInternalsAlone is the negative fixture the
// isEvoOwnTestFile/isNamedCompatTestShim exemptions (kept.go, evotype.go)
// need: every other fixture in this package type-checks against a
// throwaway *consumer* module (writeModule), so the evo package itself —
// the only place those exemptions can fire — is never exercised by
// TestAnalyzersAgainstGoldenFixtures or TestUserStepForTestStillFlagged.
// Without this test, deleting both exemptions still leaves `go test
// ./...` green; only `evident-output fix ./...` run against evo's own
// checkout would change, silently.
//
// This loads evo's own module root directly (no writeModule wrapper) and
// asserts silence, scoped to the exact rule each exemption backs, for the
// two evo-own files that call the still-exported-but-retired vocabulary
// deliberately: kept_conclusion_test.go / taxonomy_test.go pin Kept's own
// contract (API-091, package evo_test) and export_test.go's StepForTest
// compat shim calls Step (API-090, package evo). It does not assert
// silence across those whole files: taxonomy_test.go also exercises
// ForSkip/OnTask (API-120), which the vocabulary freeze retired outright
// and which this exemption never covers — those findings are expected.
func TestAnalyzersLeaveEvoOwnInternalsAlone(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot = filepath.Dir(filepath.Dir(filepath.Dir(repoRoot))) // internal/agent/fix -> repo root

	pkgs, err := fix.Load(repoRoot, "./...")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}

	// file -> rule IDs that must never fire in it (the exemption's scope),
	// not every rule the file happens to trip.
	exempt := map[string]map[string]bool{
		"kept_conclusion_test.go": {"API-091": true},
		"taxonomy_test.go":        {"API-091": true},
		"export_test.go":          {"API-090": true},
	}
	for _, result := range results {
		for _, d := range result.Diagnostics {
			base := filepath.Base(d.Filename)
			if exempt[base][d.RuleID] {
				t.Errorf("evo's own %s: unexpected %s diagnostic at line %d: %s (exemption regressed)", base, d.RuleID, d.Line, d.Message)
			}
		}
	}
}
