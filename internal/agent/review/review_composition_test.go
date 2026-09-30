package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// compositionRuleCases pairs each composition rule with its bad fixture (the
// rule must fire wantBad times) and its good fixture (the rule must not).
var compositionRuleCases = []struct {
	ruleID  string
	bad     string
	good    string
	wantBad int
}{
	{"API-064", "api_064_bad.go", "api_064_good.go", 1},
	{"API-065", "api_065_bad.go", "api_065_good.go", 1},
	{"API-066", "api_066_bad.go", "api_066_good.go", 1},
	{"API-067", "api_067_bad.go", "api_067_good.go", 1},
	{"API-068", "api_068_bad.go", "api_068_good.go", 1},
	{"API-069", "api_069_bad.go", "api_069_good.go", 4},
	{"API-070", "api_070_bad.go", "api_070_good.go", 1},
	{"API-071", "api_071_bad.go", "api_071_good.go", 1},
	{"EVO-UI-004", "evo_ui_004_bad.go", "evo_ui_004_good.go", 2},
	{"EVO-WIRE-002", "evo_wire_002_bad.go", "evo_wire_002_good.go", 1},
}

func countRule(res review.Result, ruleID string) int {
	n := 0
	for _, f := range res.Findings {
		if f.RuleID == ruleID {
			n++
		}
	}
	return n
}

func TestCompositionRules_BadFixturesFireAndGoodFixturesStaySilent(t *testing.T) {
	for _, c := range compositionRuleCases {
		t.Run(c.ruleID, func(t *testing.T) {
			bad := review.GoSource(c.bad, readFixture(t, c.bad))
			if got := countRule(bad, c.ruleID); got != c.wantBad {
				t.Fatalf("%s on %s: %d findings, want %d: %+v", c.ruleID, c.bad, got, c.wantBad, bad.Findings)
			}
			good := review.GoSource(c.good, readFixture(t, c.good))
			assertNoFinding(t, good, c.ruleID)
		})
	}
}

// TestCompositionRules_CanonicalPruneShapeIsClean is the false-positive
// guard: zq's settled prune topology triggers none of API-064..API-071.
func TestCompositionRules_CanonicalPruneShapeIsClean(t *testing.T) {
	res := review.GoSource("prune.go", readFixture(t, "composition_prune_good.go"))
	for _, id := range []string{"API-064", "API-065", "API-066", "API-067", "API-068", "API-069", "API-070", "API-071"} {
		assertNoFinding(t, res, id)
	}
}

func TestAPI064_SuggestionNamesTheMissingEdge(t *testing.T) {
	res := review.GoSource("bad.go", readFixture(t, "api_064_bad.go"))
	f := assertFinding(t, res, "API-064")
	if want := "After(branches)"; !strings.Contains(f.Suggestion, want) {
		t.Fatalf("suggestion %q must name %q", f.Suggestion, want)
	}
}

func TestAPI065_NeverSuggestsAddingAnAfterInsideASequence(t *testing.T) {
	res := review.GoSource("bad.go", readFixture(t, "api_065_bad.go"))
	f := assertFinding(t, res, "API-065")
	if !strings.Contains(f.Suggestion, "delete After(managers)") {
		t.Fatalf("suggestion must remove the edge, got %q", f.Suggestion)
	}
}

func TestCompositionRules_OlderPinDropsTheirFindings(t *testing.T) {
	res := review.GoSourceAt("bad.go", readFixture(t, "api_064_bad.go"), "1.1.0")
	assertNoFinding(t, res, "API-064")
}
