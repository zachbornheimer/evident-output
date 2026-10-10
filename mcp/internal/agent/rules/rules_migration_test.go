package rules_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
)

// spec §59: the version-transition table must be machine-readable — every
// row names both shapes and a target version, and a row whose RuleID is set
// must resolve through rules.Explain (the same closed-loop guarantee
// review's emitted IDs already carry).
func TestMigrations_EveryRowHasFromToAndSince(t *testing.T) {
	rows := rules.Migrations()
	if len(rows) == 0 {
		t.Fatal("Migrations() returned no rows")
	}
	for _, row := range rows {
		if row.From == "" || row.To == "" || row.Since == "" {
			t.Fatalf("incomplete migration row: %+v", row)
		}
		if row.RuleID == "" {
			continue
		}
		if _, ok := rules.Explain(row.RuleID); !ok {
			t.Fatalf("migration row cites rule %q but rules.Explain cannot resolve it", row.RuleID)
		}
	}
}

// The two 1.0 removals the work order calls out by name must appear so an
// upgrade assistant can find them mechanically.
func TestMigrations_CoversMainWithAndEachRemoval(t *testing.T) {
	rows := rules.Migrations()
	var sawMainWith, sawEach bool
	for _, row := range rows {
		if !row.Removed {
			continue
		}
		switch {
		case containsAll(row.From, "MainWith"):
			sawMainWith = true
			if !containsAll(row.To, "os.Exit", "evo.Main") {
				t.Fatalf("MainWith migration target missing os.Exit(evo.Main(...)): %+v", row)
			}
		case containsAll(row.From, "Each"):
			sawEach = true
			if !containsAll(row.To, "Task") {
				t.Fatalf("Each migration target missing per-item Task guidance: %+v", row)
			}
		}
	}
	if !sawMainWith {
		t.Fatal("missing removed-MainWith migration row")
	}
	if !sawEach {
		t.Fatal("missing removed-Each migration row")
	}
}

// spec §59 lists a named-Evidence-for-common-file-state row between the
// evo.File row and the manual-counters row. The From side is the stale
// shape; the To side is live 1.1 (evo.File), not the removed Evidence
// spelling.
func TestMigrations_CoversNamedEvidenceForCommonFileState(t *testing.T) {
	rows := rules.Migrations()
	for _, row := range rows {
		if containsAll(row.From, "named Evidence", "common file state") &&
			containsAll(row.To, "evo.File") {
			return
		}
	}
	t.Fatal("missing §59 migration row: named Evidence used only for common file state → evo.File")
}

// The six 1.1 removals an upgrade assistant must rewrite mechanically.
func TestMigrations_Covers1_1Removals(t *testing.T) {
	want := []struct{ from, to string }{
		{"Blockf", "Block"},
		{"Failf", "Fail"},
		{"Warn", "Problem"},
		{"Step", "Progress"},
		{"Kept", "Skipped"},
		{"Evidence", "Capture"},
	}
	rows := rules.Migrations()
	for _, w := range want {
		found := false
		for _, row := range rows {
			if !row.Removed || row.Since != "1.1.0" || row.RuleID != "API-032" {
				continue
			}
			if strings.Contains(row.From, w.from) && strings.Contains(row.To, w.to) {
				found = true
				if _, ok := rules.Explain(row.RuleID); !ok {
					t.Errorf("%s row: rules.Explain(%q) failed", w.from, row.RuleID)
				}
				break
			}
		}
		if !found {
			t.Errorf("missing 1.1 migration row %s → %s", w.from, w.to)
		}
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
