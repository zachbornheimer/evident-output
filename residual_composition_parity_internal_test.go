package evo

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestResidualComposition_PlainAndInteractiveSectionParity is release-gate
// round 6 finding 1's structural proof: the primary stream's residual and
// the live terminal's final word are one composition (plain.Stream.Residual:
// unemitted lines, unresolved entities, collections, effects, conclusion,
// debug tail). On an interactive run whose rows all streamed already, the
// two texts (modulo the final word's trailing-newline trim) must be
// identical, in color and in no-color builds. A section that reached only
// one destination breaks this test instead of silently going missing.
func TestResidualComposition_PlainAndInteractiveSectionParity(t *testing.T) {
	for _, colorOn := range []bool{false, true} {
		opts := []option{withTerminal(testkit.NewScreen(testkit.Interactive())), visibilityDelay(0), dryRun()}
		if !colorOn {
			opts = append(opts, withNoColor())
		}
		out := newOutput("parity", opts...)
		t.Cleanup(func() { _ = out.close() })

		task := out.Task("branches")
		task.fail("could not delete", Detail("permission denied"))
		cleanup := out.Task("cleanup")
		cleanup.Define(effectOf(Delete, "stale local branch", 3))
		if err := cleanup.wait(); err != nil {
			t.Fatalf("cleanup wait released: %v", err)
		}

		out.mu.Lock()
		snap := out.snapshotLocked()
		conc := record.InferConclusion(snap)
		record.FoldLeftoverMisuse(&conc, out.firstMisuse())
		record.ApplyFailedExitCode(&conc, out.cfg.failedExitCode)
		snap.Conclusion = &conc
		plain, interactive := out.stream.Residual(snap)
		out.mu.Unlock()

		gotPlain := strings.TrimRight(plain, "\n")
		gotInteractive := strings.TrimRight(interactive, "\n")
		if gotPlain != gotInteractive {
			t.Fatalf("color=%v: the residual and the final word diverged (parity property broken):\nplain:\n%q\ninteractive:\n%q",
				colorOn, gotPlain, gotInteractive)
		}
		// Sanity: the effects section (finding 1's actual gap) must be present
		// in both, not merely equal-because-both-empty.
		if !strings.Contains(gotPlain, "planned") || !strings.Contains(gotPlain, "stale local branch") {
			t.Fatalf("color=%v: parity check itself is vacuous — effects section missing:\n%q", colorOn, gotPlain)
		}
	}
}
