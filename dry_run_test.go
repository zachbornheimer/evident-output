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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	branches := out.Task("branches")
	_ = branches.Delete("local branches", nil, evo.Affected(12))
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	_ = branches.Delete("local branches", nil, evo.Affected(12))
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("retire"), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	branches := out.Task("branches")
	_ = branches.Delete("local branches", nil, evo.Affected(3))
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
	// FinalPlain is unexported (C8); reconstruct the same text RenderPlain
	// produces from the finished snapshot.
	finalPlain, err := evo.RenderPlain(out.Snapshot(), evo.PlainOptions{Width: 80, NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(finalPlain), "deleted") {
		t.Fatalf("final plain missing deleted row:\n%s", finalPlain)
	}
}

// TestTaskHandle_MultipleMutationsAccumulateOnOneSubject exercises the
// get-or-create Plan/Changes identity: repeated mutation calls on the same
// task accumulate into one section instead of one per call.
func TestTaskHandle_MultipleMutationsAccumulateOnOneSubject(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("retire"), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	branches := out.Task("branches")
	_ = branches.Delete("local branches", nil, evo.Affected(3))
	_ = branches.Update("tip", nil, evo.Affected(1))
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
			out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("t"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("t"), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("branches")
	task.Done()

	_ = task.Delete("thing", nil, evo.Affected(1))

	if out.Err() == nil {
		t.Fatal("want recorded misuse after mutating a resolved task")
	}
	snap := out.Snapshot()
	if len(snap.Changes) != 0 {
		t.Fatalf("no mutation should have been recorded, got %+v", snap.Changes)
	}
}

// TestWriteEffects_ZeroAffectedMutationVerbRendersNoSection proves a
// mutation-verb call (TaskHandle.Delete/...) with Affected(0) never declares
// a Plan/Changes section at all — no ledger row, no "nothing to X" fallback
// line either.
//
// E2.5 finding 4 supersedes this test's original expectation ("nothing to
// delete branches" via the mutation-verb boundary): that grammar is exactly
// the fixture's "[planned] repo-retire" phantom-row bug class an effect that
// never happened must never materialize a ledger section of its own. The
// "nothing to <verb> <subject>" empty-section grammar itself is still
// covered — for TaskHandle.Record's own zero-quantity contract, a distinct,
// lower-level primitive Affected's fix does not touch — by
// TestSpecP18_RemoteTrackingVsRemoteDelete_Step2.
func TestWriteEffects_ZeroAffectedMutationVerbRendersNoSection(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	branches := out.Task("branches")
	_ = branches.Delete("local branches", nil, evo.Affected(0))
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "nothing to") || strings.Contains(got, "[planned]  branches") {
		t.Fatalf("want no \"branches\" ledger section at all for a zero-Affected mutation verb, got:\n%s", got)
	}
	if len(out.Snapshot().Plans) != 0 {
		t.Fatalf("want no Plan section declared, got %+v", out.Snapshot().Plans)
	}
}
