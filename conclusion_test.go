package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestWriteCollection_DoneChildrenSurviveWithSummaries is the red-first case
// for evo-rec.md Problem 1's "Group children must survive into the final
// ledger": a Done child with a Summary used to vanish from the plain/final
// projection because writeCollection only rendered "notable" (non-Done)
// children. The final output must list every resolved child with its
// summary, exactly like the spec's "✓ branches   14 deleted" row.
func TestWriteCollection_DoneChildrenSurviveWithSummaries(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{Name: "pipeline", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	g := out.Group("pipeline")
	succeed(g.Task("branches"), "14 deleted")
	succeed(g.Task("worktrees"), "2 removed")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"branches", "14 deleted", "worktrees", "2 removed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Done child with summary must survive into the final ledger, want %q in:\n%s", want, got)
		}
	}
}

// TestConclusion_WarningDoesNotOverrideOKOutcome is the red-first case for
// evo-rec.md Problem 3: a warning row must not flip an otherwise-OK verdict
// to a contradictory "[warning]" trailer sitting right under a ✓ row — the
// reproduction was "✓ clean" immediately followed by "[warning]  repo-retire".
// Per the two-axis conclusion algebra, Outcome is OK|Blocked|Failed|Cancelled;
// warnings stay visible on their own "!" row without becoming the headline
// when a Done task already makes the outcome OK.
func TestConclusion_WarningDoesNotOverrideOKOutcome(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{Name: "repo-retire", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	succeed(out.Task("clean"))
	out.Task("kept").Fact("kept", "1")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	c := out.conclusion()
	if c.headline() == "warning" {
		t.Fatalf("warning must not override an otherwise-OK outcome, got conclusion state %v", c.headline())
	}
	got := buf.String()
	if strings.Contains(got, "[warning]") {
		t.Fatalf("trailing conclusion must not contradict the ✓ row with [warning]:\n%s", got)
	}
}

// TestConclusion_LoneIncompleteTaskIsNotPartialHeadline is the red-first
// case for evo-rec.md's "Partial is a modifier, not a root verdict": an
// unresolved task at Finish must not invent a new headline state of its own
// — Partial stays evidence (Conclusion.Partial) layered over one of the four
// Outcome states (StateReady/.../StateCancelled), never a fifth state.
// There is no StatePartial to compare against: the round-4 release gate
// killed that dead enum member (it was never assigned by inferConclusion)
// rather than keep two competing models of the same fact.
func TestConclusion_LoneIncompleteTaskIsNotPartialHeadline(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	out.Task("install") // declared, never resolved

	_ = out.finish()
	conc := out.conclusion()

	switch conc.headline() {
	case headlineSucceeded, headlineChanged, "warning",
		headlineRefused, headlineFailed, headlineCancelled, headlinePlanned:
	default:
		t.Fatalf("headline = %v, want one of the documented Outcome states", conc.headline())
	}
	if !conc.partial {
		t.Fatal("want Partial=true retained as evidence")
	}
}

func TestConclusion_PlanOnlyIsPlanned(t *testing.T) {
	out := initRun(Config{Name: "acct", DryRun: true, Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	out.Task("delete").Define(effectOf(Delete, "thing", 1))
	_ = out.finish()
	requireConclusion(t, out, headlinePlanned)
	requireClean(t, out)
}
