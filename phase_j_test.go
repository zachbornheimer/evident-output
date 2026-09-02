package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDryRun_MarkerAnnouncesRunAsFirstLine is the red-first case for
// evo-rec.md Problem 1: "a dry run must announce itself — library-owned."
// A DryRun-configured Output cannot finish without an unmissable marker
// line appearing before anything else in the durable output.
func TestDryRun_MarkerAnnouncesRunAsFirstLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun())
	branches := out.Task("branches")
	branches.Delete(12, "local branches")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "[dry-run]") {
		t.Fatalf("want dry-run marker, got:\n%s", got)
	}
	lines := strings.SplitN(got, "\n", 2)
	if !strings.Contains(lines[0], "[dry-run]") {
		t.Fatalf("dry-run marker must be the first line, got:\n%s", got)
	}
}

// TestDryRun_MarkerAbsentWhenNotDryRun pins the counterpart: an ordinary
// (non-DryRun) run never emits the marker.
func TestDryRun_MarkerAbsentWhenNotDryRun(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor())
	branches := out.Task("branches")
	branches.Delete(12, "local branches")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "[dry-run]") {
		t.Fatalf("an applied run must never render the dry-run marker:\n%s", buf.String())
	}
}

// TestDryRun_ConclusionReadsPlannedNotDone pins the second half of Problem 1:
// the trailing conclusion of a dry run must read planned-not-done, never the
// ✓/StateReady "done" form — including once a run has no Plan section of its
// own (inferConclusion must not fall through to Ready/Changed for DryRun).
func TestDryRun_ConclusionReadsPlannedNotDone(t *testing.T) {
	t.Parallel()
	out := evo.NewWithOptions(evo.Title("retire"), evo.NoColor(), evo.DryRun())
	t.Cleanup(func() { _ = out.Close() })
	branches := out.Task("branches")
	branches.Delete(12, "local branches")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StatePlanned {
		t.Fatalf("conclusion state = %v, want StatePlanned for a dry run", c.State)
	}
	if c.ExitCode != evo.ExitOK {
		t.Fatalf("exit code = %d, want unchanged ExitOK", c.ExitCode)
	}
}

// TestDryRun_ConclusionReadsPlannedEvenWithoutAPlanSection is the red-first
// case that isolates the DryRun override itself: a dry run whose only
// content is a resolved Item (no Plan/Changes section at all) would
// otherwise fall through to StateReady — DryRun must still keep the
// headline planned-not-done.
func TestDryRun_ConclusionReadsPlannedEvenWithoutAPlanSection(t *testing.T) {
	t.Parallel()
	out := evo.NewWithOptions(evo.Title("retire"), evo.NoColor(), evo.DryRun())
	t.Cleanup(func() { _ = out.Close() })
	out.Item("scan").OK()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if c := out.Conclusion(); c.State != evo.StatePlanned {
		t.Fatalf("conclusion state = %v, want StatePlanned even with no Plan section", c.State)
	}
}

// TestWriteCollection_DoneChildrenSurviveWithSummaries is the red-first case
// for evo-rec.md Problem 1's "Group children must survive into the final
// ledger": a Done child with a Summary used to vanish from the plain/final
// projection because writeCollection only rendered "notable" (non-Done)
// children. The final output must list every resolved child with its
// summary, exactly like the spec's "✓  branches   14 deleted" row.
func TestWriteCollection_DoneChildrenSurviveWithSummaries(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor())
	g := out.Tasks("pipeline")
	g.Task("branches").Done("14 deleted")
	g.Task("worktrees").Done("2 removed")
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
// reproduction was "✓  clean" immediately followed by "[warning]  repo-retire".
// Per the two-axis conclusion algebra, Outcome is OK|Blocked|Failed|Cancelled;
// warnings stay visible on their own "!" row without becoming the headline
// when a Done task already makes the outcome OK.
func TestConclusion_WarningDoesNotOverrideOKOutcome(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("repo-retire"), evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Task("clean").Done()
	out.Item("kept").Warn("kept 1")
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

// TestConclusion_WarningOnlyStillReadsWarning is the green counterpart:
// when a warning is the only content in the run (nothing else to be OK
// about), the conclusion still reads StateWarning — moving warning below
// Done/Changed/Planned in headline precedence must not erase this case.
func TestConclusion_WarningOnlyStillReadsWarning(t *testing.T) {
	t.Parallel()
	out := evo.NewWithOptions(evo.Title("t"), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	out.Item("i").Warn("careful")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if out.Conclusion().State != evo.StateWarning {
		t.Fatalf("conclusion state = %v, want StateWarning when nothing else in the run resolved OK", out.Conclusion().State)
	}
}

// TestConformance_Problem1SuccessBlock renders evo-rec.md's Problem 1
// "success" block shape end-to-end via the public API and asserts the
// durable output matches the dialect: glyph column, counts, and derived
// skip/keep taxonomy lines.
//
//	✓  branches   14 deleted
//	✓  worktrees  2 removed
//	!  skipped 6  (...)
//	!  kept 3     (...)
func TestConformance_Problem1SuccessBlock(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault mutates process-global state, same as
	// the existing default-instance tests in taxonomy_test.go.
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()))

	branches := evo.Task("branches")
	protected := evo.Reason("protected")
	dirty := evo.Reason("dirty")
	unpushed := evo.Reason("unpushed")
	branches.Skipped(protected, "main")
	branches.Skipped(dirty, "feat/wip-1")
	branches.Skipped(dirty, "feat/wip-2")
	branches.Skipped(dirty, "feat/wip-3")
	branches.Skipped(dirty, "feat/wip-4")
	branches.Skipped(dirty, "feat/wip-5")
	branches.Kept(unpushed, "feat/a")
	branches.Kept(unpushed, "feat/b")
	branches.Kept(unpushed, "feat/c")
	branches.Delete(14, "local branches")
	branches.Done()

	worktrees := evo.Task("worktrees")
	worktrees.Remove(2, "worktrees")
	worktrees.Done()

	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	// Glyph column + counts, per task (the [changed] mutation ledger, not a
	// caller-composed string — see task_mutations.go).
	for _, want := range []string{
		"[changed]  branches",
		"deleted  14 local branches",
		"[changed]  worktrees",
		"removed  2 worktrees",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in dialect-conformant output:\n%s", want, got)
		}
	}
	// Derived taxonomy lines: counts sum from the accumulated records, never
	// hand-assembled (TAX-001).
	if !strings.Contains(got, "skipped 6  (1 protected, 5 dirty)") {
		t.Fatalf("want summed skip taxonomy, got:\n%s", got)
	}
	if !strings.Contains(got, "kept 3  (unpushed)") {
		t.Fatalf("want kept taxonomy, got:\n%s", got)
	}
}
