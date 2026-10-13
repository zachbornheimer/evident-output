package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestDryRun_TrueRendersPlannedImperative is the red-first case for the
// caller-never-writes-tense contract: with Config.DryRun true, a mutation
// verb renders under [planned] with the imperative verb the caller wrote.
func TestDryRun_TrueRendersPlannedImperative(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, title: "retire", color: colorNever, plain: true, DryRun: true})
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local branch", 12))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "[planned] branches") {
		t.Fatalf("want [planned] section, got:\n%s", got)
	}
	if strings.Contains(got, "[changed]") {
		t.Fatalf("dry run must never render [changed]:\n%s", got)
	}
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "delete 12 local branches") {
		t.Fatalf("want imperative 'delete', got:\n%s", got)
	}
	if strings.Contains(collapsed, "deleted") {
		t.Fatalf("dry run must not conjugate to past tense:\n%s", got)
	}
}

// TestDryRun_FalseRendersChangedPastTense is the green counterpart: the same
// call site, with DryRun false, renders [changed] and the conjugated verb.
func TestDryRun_FalseRendersChangedPastTense(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, title: "retire", color: colorNever, plain: true})
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local branch", 12))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "[changed] branches") {
		t.Fatalf("want [changed] section, got:\n%s", got)
	}
	if strings.Contains(got, "[planned]") {
		t.Fatalf("apply run must never render [planned]:\n%s", got)
	}
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "deleted 12 local branches") {
		t.Fatalf("want past-tense 'deleted', got:\n%s", got)
	}
}

// TestTaskHandle_DeleteForwardsToChangesLedger verifies the mutation verb
// reaches the Changes/Plan machinery (not a separate ad hoc ledger) and
// therefore appears in the snapshot and FinalPlain.
func TestTaskHandle_DeleteForwardsToChangesLedger(t *testing.T) {
	t.Parallel()
	out := initRun(Config{isolated: true, title: "retire", color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local branch", 3))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.snapshot()
	if len(snap.Changes) != 1 {
		t.Fatalf("changes sections = %d, want 1", len(snap.Changes))
	}
	if snap.Changes[0].Subject != "branches" {
		t.Fatalf("subject = %q, want branches", snap.Changes[0].Subject)
	}
	if len(snap.Changes[0].Records) != 1 || snap.Changes[0].Records[0].Verb != "deleted" {
		t.Fatalf("records = %+v", snap.Changes[0].Records)
	}
	// FinalPlain is unexported (C8); reconstruct the same text RenderPlain
	// produces from the finished snapshot.
	finalPlain, err := renderPlain_1(out.snapshot(), plainOptions{width: 80, noColor: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(finalPlain), "deleted") {
		t.Fatalf("final plain missing deleted row:\n%s", finalPlain)
	}
}

// TestTaskHandle_MultipleMutationsAccumulateOnOneSubject exercises the
// get-or-create Plan/Changes identity: repeated Effect calls in one Task's
// Define accumulate into one section instead of one per call.
func TestTaskHandle_MultipleMutationsAccumulateOnOneSubject(t *testing.T) {
	t.Parallel()
	out := initRun(Config{isolated: true, title: "retire", color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	commit(out.Task("branches"),
		EffectSpec{Verb: Delete, Object: "local branch", Quantity: 3},
		EffectSpec{Verb: Update, Object: "tip", Quantity: 1},
	)
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.snapshot()
	if len(snap.Changes) != 1 {
		t.Fatalf("changes sections = %d, want 1", len(snap.Changes))
	}
	if len(snap.Changes[0].Records) != 2 {
		t.Fatalf("records = %+v, want 2", snap.Changes[0].Records)
	}
}

// TestConjugatePast_TableIncludingIrregulars pins the display-facing tense
// conjugation of every EffectVerb: the default +d/+ed rule, the doubled
// consonant, and install/uninstall. (write->wrote belongs to File.)
func TestConjugatePast_TableIncludingIrregulars(t *testing.T) {
	t.Parallel()
	cases := map[effectVerb]string{
		effectAdd:       "added",
		effectDelete:    "deleted",
		effectCreate:    "created",
		effectUpdate:    "updated",
		effectRemove:    "removed",
		effectPush:      "pushed",
		effectInstall:   "installed",
		effectUninstall: "uninstalled",
	}
	for imperative, want := range cases {
		t.Run(string(imperative), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			out := initRun(Config{isolated: true, stdout: &buf, title: "t", color: colorNever, plain: true})
			commit(out.Task("subject"), EffectSpec{Verb: Verb(imperative), Object: "object", Quantity: 1})
			if err := out.finish(); err != nil {
				t.Fatal(err)
			}
			snap := out.snapshot()
			if len(snap.Changes) != 1 || snap.Changes[0].Records[0].Verb != want {
				t.Fatalf("%s -> %v, want %s", imperative, snap.Changes, want)
			}
		})
	}
}

// TestTaskHandle_MutationOnResolvedTaskRecordsMisuse is the red-first case for
// the "resolved tasks record misuse, never panic" contract.
func TestTaskHandle_MutationOnResolvedTaskRecordsMisuse(t *testing.T) {
	t.Parallel()
	out := initRun(Config{isolated: true, title: "t", color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("branches")
	succeed(task)

	task.Define(effectOf(Delete, "thing", 1))

	if out.err() == nil {
		t.Fatal("want recorded misuse after mutating a resolved task")
	}
	snap := out.snapshot()
	if len(snap.Changes) != 0 {
		t.Fatalf("no mutation should have been recorded, got %+v", snap.Changes)
	}
}

// TestDryRun_MarkerAnnouncesRunAsFirstLine is the red-first case for
// evo-rec.md Problem 1: "a dry run must announce itself — library-owned."
// A DryRun-configured Output cannot finish without an unmissable marker
// line appearing before anything else in the durable output.
func TestDryRun_MarkerAnnouncesRunAsFirstLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "retire", color: colorNever, plain: true, DryRun: true})
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local branch", 12))
	if err := out.finish(); err != nil {
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
	out := initRun(Config{stdout: &buf, title: "retire", color: colorNever, plain: true})
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local branch", 12))
	if err := out.finish(); err != nil {
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
	out := initRun(Config{title: "retire", color: colorNever, DryRun: true})
	t.Cleanup(func() { _ = out.close() })
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local branch", 12))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	c := out.conclusion()
	if c.state != statePlanned {
		t.Fatalf("conclusion state = %v, want StatePlanned for a dry run", c.state)
	}
	if c.exitCode != exitOK {
		t.Fatalf("exit code = %d, want unchanged ExitOK", c.exitCode)
	}
}

// TestDryRun_ConclusionReadsPlannedEvenWithoutAPlanSection is the red-first
// case that isolates the DryRun override itself: a dry run whose only
// content is a resolved Item (no Plan/Changes section at all) would
// otherwise fall through to StateReady — DryRun must still keep the
// headline planned-not-done.
func TestDryRun_ConclusionReadsPlannedEvenWithoutAPlanSection(t *testing.T) {
	t.Parallel()
	out := initRun(Config{title: "retire", color: colorNever, DryRun: true})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("scan"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if c := out.conclusion(); c.state != statePlanned {
		t.Fatalf("conclusion state = %v, want StatePlanned even with no Plan section", c.state)
	}
}
