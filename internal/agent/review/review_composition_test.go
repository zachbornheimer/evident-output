package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// compositionRuleCases lists each composition rule and how often it must fire
// on its bad fixture. Fixture file names derive from the rule id (see
// compositionFixtures), so the table holds no file-name strings.
var compositionRuleCases = []struct {
	ruleID  string
	wantBad int
}{
	{"API-064", 1},
	{"API-065", 1},
	{"API-066", 1},
	{"API-067", 1},
	{"API-068", 1},
	{"API-069", 4},
	{"API-070", 1},
	{"API-071", 1},
	{"EVO-UI-004", 2},
	{"EVO-WIRE-002", 1},
}

// compositionFixtures names the bad and good fixture of a rule: the lower-case
// rule id with underscores, then _bad.go or _good.go.
func compositionFixtures(ruleID string) (bad, good string) {
	slug := strings.ToLower(strings.ReplaceAll(ruleID, "-", "_"))
	return slug + "_bad.go", slug + "_good.go"
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
			badName, goodName := compositionFixtures(c.ruleID)
			bad := review.GoSource(badName, readFixture(t, badName))
			if got := countRule(bad, c.ruleID); got != c.wantBad {
				t.Fatalf("%s on %s: %d findings, want %d: %+v", c.ruleID, badName, got, c.wantBad, bad.Findings)
			}
			good := review.GoSource(goodName, readFixture(t, goodName))
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
