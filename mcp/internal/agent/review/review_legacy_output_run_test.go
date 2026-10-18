package review_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// legacyOutputRunFixtures are the two shapes of the v1.0.0 hosted program:
// out := evo.Init(cfg); out.Run(ctx, run) with the callback a literal and a
// package func.
var legacyOutputRunFixtures = []string{
	"dialect_2_0/detectRemovedTwoZero/legacy_output_run.go",
	"dialect_2_0/detectRemovedTwoZero/legacy_output_run_named.go",
}

// TestAssignedInitAndOutputRunRewriteToExecute proves the hosted 1.x form
// migrates through review alone: Output.Run carries an applicable rewrite to
// evo.Execute, the Init rewrite deletes the whole assignment so the file
// still parses, and the applied program type-checks against 2.0 and
// re-reviews clean.
func TestAssignedInitAndOutputRunRewriteToExecute(t *testing.T) {
	for _, fixture := range legacyOutputRunFixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			src := readConvergenceFixture(t, filepath.Join("testdata", filepath.FromSlash(fixture)))
			findings := review.GoSource("main.go", src).Findings

			var sawRun bool
			for _, f := range findings {
				if !isApplicable(src, f) {
					t.Errorf("%s L%d has no applicable rewrite: %s", f.RuleID, f.Line, f.Suggestion)
				}
				sawRun = sawRun || strings.Contains(f.Message, "Output.Run(ctx, run) was removed in 2.0")
			}
			if !sawRun {
				t.Errorf("review reports nothing about Output.Run:\n%s", describeFindings(findings))
			}

			rewritten := applyAllSuggestions(t, src, findings, nil)
			if !strings.Contains(rewritten, "evo.Execute(context.Background(), evo.Config{Name: \"hosted\"}") {
				t.Errorf("the rewritten program does not run through evo.Execute with the Init Config:\n%s", rewritten)
			}
			requireTypeChecksAgainstTwoZero(t, rewritten, "")
			if again := review.GoSource("main.go", rewritten); len(again.Findings) != 0 || again.RecheckRequired {
				t.Fatalf("review is not clean after applying every suggestion:\n%s\nrewritten source:\n%s", describeFindings(again.Findings), rewritten)
			}
		})
	}
}
