package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDryRun_TrueRendersPlannedImperative is the red-first case for the
// caller-never-writes-tense contract: with Config.DryRun true, a mutation
// verb renders under [planned] with the imperative verb the caller wrote.
func TestDryRun_TrueRendersPlannedImperative(t *testing.T) {
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
	if !strings.Contains(got, "[planned]  branches") {
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
	out := evo.NewWithOptions(evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor())
	branches := out.Task("branches")
	branches.Delete(12, "local branches")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "[changed]  branches") {
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
	out := evo.NewWithOptions(evo.Title("retire"), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	branches := out.Task("branches")
	branches.Delete(3, "local branches")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if len(snap.Changes) != 1 {
		t.Fatalf("changes sections = %d, want 1", len(snap.Changes))
	}
	if snap.Changes[0].Subject != "branches" {
		t.Fatalf("subject = %q, want branches", snap.Changes[0].Subject)
	}
	if len(snap.Changes[0].Records) != 1 || snap.Changes[0].Records[0].Verb != "deleted" {
		t.Fatalf("records = %+v", snap.Changes[0].Records)
	}
	if !strings.Contains(out.FinalPlain(), "deleted") {
		t.Fatalf("FinalPlain missing deleted row:\n%s", out.FinalPlain())
	}
}

// TestTaskHandle_MultipleMutationsAccumulateOnOneSubject exercises the
// get-or-create Plan/Changes identity: repeated mutation calls on the same
// task accumulate into one section instead of one per call.
func TestTaskHandle_MultipleMutationsAccumulateOnOneSubject(t *testing.T) {
	t.Parallel()
	out := evo.NewWithOptions(evo.Title("retire"), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	branches := out.Task("branches")
	branches.Delete(3, "local branches")
	branches.Update(1, "tip")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if len(snap.Changes) != 1 {
		t.Fatalf("changes sections = %d, want 1", len(snap.Changes))
	}
	if len(snap.Changes[0].Records) != 2 {
		t.Fatalf("records = %+v, want 2", snap.Changes[0].Records)
	}
}

// TestConjugatePast_TableIncludingIrregulars pins the display-facing tense
// conjugation: default +d/+ed rule plus the write->wrote irregular.
func TestConjugatePast_TableIncludingIrregulars(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"delete": "deleted",
		"create": "created",
		"update": "updated",
		"remove": "removed",
		"push":   "pushed",
		"write":  "wrote",
	}
	for imperative, want := range cases {
		imperative, want := imperative, want
		t.Run(imperative, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			out := evo.NewWithOptions(evo.Title("t"), evo.To(&buf), evo.Plain(), evo.NoColor())
			subject := out.Task("subject")
			subject.RecordName(imperative, "object")
			subject.Done()
			if err := out.Finish(); err != nil {
				t.Fatal(err)
			}
			snap := out.Snapshot()
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
	out := evo.NewWithOptions(evo.Title("t"), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("branches")
	task.Done()

	task.Delete(1, "thing")

	if out.Err() == nil {
		t.Fatal("want recorded misuse after mutating a resolved task")
	}
	snap := out.Snapshot()
	if len(snap.Changes) != 0 {
		t.Fatalf("no mutation should have been recorded, got %+v", snap.Changes)
	}
}

// TestWriteEffects_EmptySectionRendersNothingToLine is the empty-case
// contract: a Plan/Changes section that ends with zero records renders an
// honest "nothing to ..." line instead of an empty [planned]/[changed]
// header. A section declared directly (never through a TaskHandle mutation
// verb) never learned a verb, so it falls back to "nothing to change for
// <subject>" — never the bare subject noun standing in for a verb
// (evo-rec.md "Empty effect section grammar").
func TestWriteEffects_EmptySectionRendersNothingToLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Plan("branches")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "nothing to change for branches") {
		t.Fatalf("want empty-success line, got:\n%s", got)
	}
	if strings.Contains(got, "[planned]  branches") {
		t.Fatalf("empty section must not render a header:\n%s", got)
	}
}

// TestWriteEffects_EmptySectionWithKnownVerb proves a section whose mutation
// verb was recorded (via TaskHandle) but which ends with zero rows — because
// every call recorded a zero quantity — keeps that verb in its "nothing to"
// line instead of falling back to the generic "change" phrasing.
func TestWriteEffects_EmptySectionWithKnownVerb(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun())
	branches := out.Task("branches")
	branches.Delete(0, "local branches")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "nothing to delete branches") {
		t.Fatalf("want verb-aware empty-success line, got:\n%s", got)
	}
}
