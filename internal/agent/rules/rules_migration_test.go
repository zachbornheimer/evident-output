package rules_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
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

// spec §59 lists exactly seven version-transition rows; the fourth (named
// Evidence used only for common file state → derived Evidence from tracked
// file state) sits between the evo.File row and the manual-counters row and
// must not be dropped when the table gains 1.0-specific removal rows.
func TestMigrations_CoversNamedEvidenceForCommonFileState(t *testing.T) {
	rows := rules.Migrations()
	for _, row := range rows {
		if containsAll(row.From, "named Evidence", "common file state") &&
			containsAll(row.To, "derived Evidence", "tracked file state") {
			return
		}
	}
	t.Fatal("missing §59 migration row: named Evidence used only for common file state → derived Evidence from tracked file state")
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
