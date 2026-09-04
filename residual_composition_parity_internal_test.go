package evo

import (
	"strings"
	"testing"
)

// TestResidualComposition_PlainAndInteractiveSectionParity is release-gate
// round 6 finding 1's structural proof: residualPlainLocked and
// residualInteractiveFinalLocked both call the one shared
// residualCompositionLocked sequence now (unemitted lines, unresolved
// entities, collections, effects, conclusion, debug tail). On a
// non-interactive Output, both destinations own rendering the same
// entities, so the two texts (modulo residualInteractiveFinalLocked's own
// trailing-newline trim — its write-target formatting, not a section
// difference) must be identical, in color and in no-color builds. A future
// section hand-added to only one of the two wrapper functions — instead of
// to residualCompositionLocked — breaks this test instead of silently
// reaching only one destination.
func TestResidualComposition_PlainAndInteractiveSectionParity(t *testing.T) {
	for _, colorOn := range []bool{false, true} {
		opts := []Option{Plain(), VisibilityDelay(0)}
		if !colorOn {
			opts = append(opts, NoColor())
		}
		out := newOutput("parity", opts...)
		t.Cleanup(func() { _ = out.Close() })

		task := out.Task("branches")
		task.Fail("could not delete", Detail("permission denied"))
		out.Plan("branches").Delete(3, "stale local branch")

		out.mu.Lock()
		snap := out.snapshotLocked()
		conc := inferConclusion(snap)
		foldLeftoverMisuseLocked(&conc, out.misuse)
		applyFailedExitCode(&conc, out.cfg.failedExitCode)
		snap.Conclusion = &conc
		linesFrom := out.linesEmitted

		plain := out.residualPlainLocked(snap)
		interactive := out.residualInteractiveFinalLocked(snap, linesFrom)
		out.mu.Unlock()

		gotPlain := strings.TrimRight(plain, "\n")
		gotInteractive := strings.TrimRight(interactive, "\n")
		if gotPlain != gotInteractive {
			t.Fatalf("color=%v: residualPlainLocked and residualInteractiveFinalLocked diverged (parity property broken):\nplain:\n%q\ninteractive:\n%q",
				colorOn, gotPlain, gotInteractive)
		}
		// Sanity: the effects section (finding 1's actual gap) must be present
		// in both, not merely equal-because-both-empty.
		if !strings.Contains(gotPlain, "planned") || !strings.Contains(gotPlain, "stale local branch") {
			t.Fatalf("color=%v: parity check itself is vacuous — effects section missing:\n%q", colorOn, gotPlain)
		}
	}
}
