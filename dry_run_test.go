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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "retire", Color: evo.ColorNever, Plain: true, DryRun: true})
	branches := out.Task("branches")
	branches.Define(effectOf(evo.EffectDelete, "local branch", 12))
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "retire", Color: evo.ColorNever, Plain: true})
	branches := out.Task("branches")
	branches.Define(effectOf(evo.EffectDelete, "local branch", 12))
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Isolated: true, Title: "retire", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })
	branches := out.Task("branches")
	branches.Define(effectOf(evo.EffectDelete, "local branch", 3))
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
// get-or-create Plan/Changes identity: repeated Effect calls in one Task's
// Define accumulate into one section instead of one per call.
func TestTaskHandle_MultipleMutationsAccumulateOnOneSubject(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Title: "retire", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })
	commit(out.Task("branches"),
		evo.EffectSpec{Verb: evo.EffectDelete, Object: "local branch", Quantity: 3},
		evo.EffectSpec{Verb: evo.EffectUpdate, Object: "tip", Quantity: 1},
	)
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
// conjugation of every EffectVerb: the default +d/+ed rule, the doubled
// consonant, and install/uninstall. (write->wrote belongs to evo.File.)
func TestConjugatePast_TableIncludingIrregulars(t *testing.T) {
	t.Parallel()
	cases := map[evo.EffectVerb]string{
		evo.EffectAdd:       "added",
		evo.EffectDelete:    "deleted",
		evo.EffectCreate:    "created",
		evo.EffectUpdate:    "updated",
		evo.EffectRemove:    "removed",
		evo.EffectPush:      "pushed",
		evo.EffectInstall:   "installed",
		evo.EffectUninstall: "uninstalled",
	}
	for imperative, want := range cases {
		t.Run(string(imperative), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "t", Color: evo.ColorNever, Plain: true})
			commit(out.Task("subject"), evo.EffectSpec{Verb: imperative, Object: "object", Quantity: 1})
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
	out := evo.Init(evo.Config{Isolated: true, Title: "t", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("branches")
	succeed(task)

	task.Define(effectOf(evo.EffectDelete, "thing", 1))

	if out.Err() == nil {
		t.Fatal("want recorded misuse after mutating a resolved task")
	}
	snap := out.Snapshot()
	if len(snap.Changes) != 0 {
		t.Fatalf("no mutation should have been recorded, got %+v", snap.Changes)
	}
}
