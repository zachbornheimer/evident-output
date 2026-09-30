package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
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
	out := evo.Init(evo.Config{Stdout: &buf, Title: "pipeline", Color: evo.ColorNever, Plain: true})
	g := out.Group("pipeline")
	succeed(g.Task("branches"), "14 deleted")
	succeed(g.Task("worktrees"), "2 removed")
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Stdout: &buf, Title: "repo-retire", Color: evo.ColorNever, Plain: true})
	succeed(out.Task("clean"))
	out.Task("kept").Fact("kept", "1")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State == evo.StateWarning {
		t.Fatalf("warning must not override an otherwise-OK outcome, got conclusion state %v", c.State)
	}
	got := buf.String()
	if strings.Contains(got, "[warning]") {
		t.Fatalf("trailing conclusion must not contradict the ✓ row with [warning]:\n%s", got)
	}
}

// TestConclusion_WarnOnlyAutoResolvesDoneAndStaysWarned is
// TestConclusion_WarningOnlyStillReadsWarning's P2 replacement: Warn no
// longer resolves its task (13-problem doc P2), so a task that only ever
// calls Warn auto-resolves Done at Finish (the same amnesty a recorded
// effect or sealed progress already gets) — the run reads StateReady, with
// Conclusion.Warned still true so the warning stays visible.
func TestConclusion_WarnOnlyAutoResolvesDoneAndStaysWarned(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Title: "t", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("i").Problem("careful", evo.Severity(evo.SeverityWarning))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.Conclusion().State; got != evo.StateReady {
		t.Fatalf("conclusion state = %v, want StateReady (Warn auto-resolves Done, P2)", got)
	}
	if !out.Conclusion().Warned {
		t.Fatal("Conclusion.Warned = false, want true: the recorded warning must stay visible")
	}
}

// TestConclusion_LoneIncompleteTaskIsNotPartialHeadline is the red-first
// case for evo-rec.md's "Partial is a modifier, not a root verdict": an
// unresolved task at Finish must not invent a new headline state of its own
// — Partial stays evidence (Conclusion.Partial) layered over one of the four
// Outcome states (evo.StateReady/.../StateCancelled), never a fifth state.
// There is no evo.StatePartial to compare against: the round-4 release gate
// killed that dead enum member (it was never assigned by inferConclusion)
// rather than keep two competing models of the same fact.
func TestConclusion_LoneIncompleteTaskIsNotPartialHeadline(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	out.Task("install") // declared, never resolved

	_ = out.Finish()
	conc := out.Conclusion()

	switch conc.State {
	case evo.StateReady, evo.StateChanged, evo.StateWarning,
		evo.StateBlocked, evo.StateFailed, evo.StateCancelled, evo.StatePlanned:
	default:
		t.Fatalf("headline = %v, want one of the documented Outcome states", conc.State)
	}
	if !conc.Partial {
		t.Fatal("want Partial=true retained as evidence")
	}
}

// TestConclusion_WarnedGroupChildReachesConclusion proves a warned-but-Done
// child nested inside a container still surfaces the "· warned" modifier on
// the run's own conclusion band, and the container's own success summary is
// suppressed rather than papering over the warning underneath it.
func TestConclusion_WarnedGroupChildReachesConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("dependencies")
	child := group.Task("cache")
	child.Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))
	succeed(child)

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	conc := out.Conclusion()
	if conc.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady", conc.State)
	}
	if !conc.Warned {
		t.Fatal("Conclusion.Warned = false, want true (group-child warning must reach the conclusion)")
	}
	if conc.ExitCode != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d", conc.ExitCode, evo.ExitOK)
	}
	if !strings.Contains(buf.String(), "[ready · warned]") {
		t.Fatalf("want the \"[ready · warned]\" conclusion band, got:\n%s", buf.String())
	}
}
