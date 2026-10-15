package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// compositionRuleCases lists each composition rule and how often it must fire
// on its bad fixture. A rule with a detector reads bad.go and good.go from
// testdata/dialect_2_0/<detector>/, the compiled 2.0 programs
// TestEveryDetectorHasA20Fixture also checks; a rule without one reads the
// testdata/evo_rules pair named from its rule id (see compositionFixtures).
var compositionRuleCases = []struct {
	ruleID   string
	detector string
	wantBad  int
}{
	{"API-064", "detectUnorderedComputedGet", 1},
	{"API-065", "detectRedundantAfterInSequence", 1},
	{"API-066", "detectOuterVariableHandoff", 1},
	{"API-068", "detectChildrenDeclaredInTaskDefine", 1},
	{"API-069", "detectImpureTopologyBuilder", 3},
	{"API-070", "detectCategoryTaskLoopingOverItems", 1},
	{"API-071", "detectPresentationOnlyTask", 1},
	{"EVO-UI-004", "", 2},
	{"EVO-WIRE-002", "", 1},
}

// compositionFixtures names the bad and good fixture of a rule: the lower-case
// rule id with underscores, then _bad.go or _good.go.
func compositionFixtures(ruleID string) (bad, good string) {
	slug := strings.ToLower(strings.ReplaceAll(ruleID, "-", "_"))
	return slug + "_bad.go", slug + "_good.go"
}

// readDialectFixture loads one program of a detector's 2.0 fixture directory.
func readDialectFixture(t *testing.T, detector, name string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "dialect_2_0", detector, name))
	if err != nil {
		t.Fatalf("read fixture %s/%s: %v", detector, name, err)
	}
	return string(src)
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
			badSrc, goodSrc := "", ""
			if c.detector == "" {
				badSrc, goodSrc = readFixture(t, badName), readFixture(t, goodName)
			} else {
				badName, goodName = "bad.go", "good.go"
				badSrc, goodSrc = readDialectFixture(t, c.detector, badName), readDialectFixture(t, c.detector, goodName)
			}
			bad := review.GoSource(badName, badSrc)
			if got := countRule(bad, c.ruleID); got != c.wantBad {
				t.Fatalf("%s on %s: %d findings, want %d: %+v", c.ruleID, badName, got, c.wantBad, bad.Findings)
			}
			good := review.GoSource(goodName, goodSrc)
			assertNoFinding(t, good, c.ruleID)
		})
	}
}

// TestCompositionRules_CanonicalPruneShapeIsClean is the false-positive
// guard: zq's settled prune topology triggers none of the composition rules.
func TestCompositionRules_CanonicalPruneShapeIsClean(t *testing.T) {
	res := review.GoSource("prune.go", readDialectFixture(t, "detectUnorderedComputedGet", "good_prune.go"))
	for _, id := range []string{"API-039", "API-064", "API-065", "API-066", "API-068", "API-069", "API-070", "API-071"} {
		assertNoFinding(t, res, id)
	}
}

// A Build callback runs as work of its Group, so a Get inside it needs the
// Group ordered after the producer just as a Task callback does.
func TestAPI064_UnorderedGetInsideBuildIsFlagged(t *testing.T) {
	res := review.GoSource("bad.go", readDialectFixture(t, "detectUnorderedComputedGet", "bad_in_builder.go"))
	f := assertFinding(t, res, "API-064")
	if want := "After(branches)"; !strings.Contains(f.Suggestion, want) {
		t.Fatalf("suggestion %q must name %q", f.Suggestion, want)
	}
}

func TestAPI064_SuggestionNamesTheMissingEdge(t *testing.T) {
	res := review.GoSource("bad.go", readDialectFixture(t, "detectUnorderedComputedGet", "bad.go"))
	f := assertFinding(t, res, "API-064")
	if want := "After(branches)"; !strings.Contains(f.Suggestion, want) {
		t.Fatalf("suggestion %q must name %q", f.Suggestion, want)
	}
}

func TestAPI065_NeverSuggestsAddingAnAfterInsideASequence(t *testing.T) {
	res := review.GoSource("bad.go", readDialectFixture(t, "detectRedundantAfterInSequence", "bad.go"))
	f := assertFinding(t, res, "API-065")
	if !strings.HasPrefix(f.Suggestion, "delete .After(managers); ") {
		t.Fatalf("suggestion must remove the whole selector call including its dot, got %q", f.Suggestion)
	}
}

// An After that also names an unrelated Task keeps that edge: the rewrite
// replaces the call, it does not delete it.
func TestAPI065_KeepsTheEdgesTheSequenceDoesNotOrder(t *testing.T) {
	src := strings.Replace(readDialectFixture(t, "detectRedundantAfterInSequence", "bad.go"),
		".After(managers)", ".After(managers, other)", 1)
	src = strings.Replace(src, "func detect(", "var other *evo.Task\n\nfunc detect(", 1)
	f := assertFinding(t, review.GoSource("bad.go", src), "API-065")
	if want := "replace .After(managers, other) with .After(other)"; f.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", f.Suggestion, want)
	}
}

func TestCompositionRules_OlderPinDropsTheirFindings(t *testing.T) {
	res := review.GoSourceAt("bad.go", readDialectFixture(t, "detectUnorderedComputedGet", "bad.go"), "1.1.0")
	assertNoFinding(t, res, "API-064")
}

// A Get inside a Task whose container is declared After(computed) is
// ordered by the container's edge; the runtime accepts it.
func TestAPI064_HonoursContainerAfter(t *testing.T) {
	src := `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	n := evo.Compute(r.Task("count"), count)
	steps := r.Sequence("steps").After(n)
	steps.Task("report").Define(func(ctx context.Context) error {
		return show(n.Get())
	})
}
`
	assertNoFinding(t, review.GoSource("container_after.go", src), "API-064")
}
