package freshness

import (
	"path/filepath"
	"testing"
)

func pathBasis(key string) []BasisRecord {
	return []BasisRecord{{Kind: string(KindFSPath), Key: key}}
}

func TestPlanChangesAPlannedPathAndEveryDirectoryAboveIt(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out")
	planned := filepath.Join(out, "o.yaml")
	table := NewTaskTable()
	plan := table.Plan()
	if plan.Changes(pathBasis(planned)) {
		t.Fatal("a Plan with nothing planned changes a path")
	}

	table.PlanOperation("producer", planned)

	for key, want := range map[string]bool{
		planned:                          true,
		out:                              true,
		root:                             true,
		filepath.Join(root, "other"):     false,
		filepath.Join(out, "o.yaml.bak"): false,
	} {
		if got := plan.Changes(pathBasis(key)); got != want {
			t.Errorf("Changes(%s) = %v, want %v", key, got, want)
		}
	}
	if plan.Changes([]BasisRecord{{Kind: string(KindValue), Key: planned}}) {
		t.Error("a value Basis named like a planned path is a change")
	}
	if (Plan{}).Changes(pathBasis(planned)) {
		t.Error("a Run that applies its work judged a planned change")
	}
}

func TestEvaluatePlansACurrentExecWhoseBasisAPlannedOperationChanges(t *testing.T) {
	dir := t.TempDir()
	input, output := filepath.Join(dir, "o.yaml"), filepath.Join(dir, "y.yaml")
	writeFile(t, input, "abc")
	writeFile(t, output, "abc")
	basis, err := basisRecordsFrom(t.Context(), []Fingerprint{FSPath(input)})
	if err != nil {
		t.Fatal(err)
	}
	call := ExecCall{ExecutablePath: input, Args: []string{"cp"}, Dir: dir, Basis: basis, Outputs: []string{output}}
	table := NewTaskTable()
	first, err := call.Evaluate(t.Context(), commitOperations(t, "task"), "task", 0, Plan{})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := PathOutputDigest(t.Context(), output)
	if err != nil {
		t.Fatal(err)
	}
	store := commitOperations(t, "task", OperationRecord{
		Kind: OperationKindExec, DefinitionFingerprint: first.DefinitionFingerprint, Basis: basis,
		Outputs: []OutputRecord{{Kind: OutputKindExec, Path: output, Digest: digest}},
	})

	if current, err := call.Evaluate(t.Context(), store, "task", 0, table.Plan()); err != nil || current.Disposition != ExecIsCurrent {
		t.Fatalf("Evaluate before anything is planned = %+v, %v, want current", current, err)
	}

	table.PlanOperation("producer", input)
	got, err := call.Evaluate(t.Context(), store, "task", 0, table.Plan())
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposition != ExecIsPlanned || got.Reason != ReasonPlannedInput {
		t.Fatalf("Evaluate = %+v, want planned for %s", got, ReasonPlannedInput)
	}
}

// A planned Run never judges a Task's Basis current when an operation it
// planned earlier would change a Basis path.
func TestPlannedRunJudgesABasisAPlannedOperationChangesNotCurrent(t *testing.T) {
	state, input := t.TempDir(), filepath.Join(t.TempDir(), "o.yaml")
	writeFile(t, input, "abc")
	task := testTask{id: "t1", key: "k"}
	applied, appliedTable := sessionOn(state), NewTaskTable()
	appliedTable.AddInputs(task.id, FingerprintBasis(FSPath(input)))
	if _, err := appliedTable.JudgeBasis(t.Context(), applied.Store, task, RunApplies); err != nil {
		t.Fatal(err)
	}
	if commit := appliedTable.Commit(t.Context(), applied, task, RunApplies); !commit.Written || commit.Err != nil {
		t.Fatalf("commit = %+v", commit)
	}
	if err := applied.Save(); err != nil {
		t.Fatal(err)
	}
	if err := applied.Close(); err != nil {
		t.Fatal(err)
	}

	for _, planned := range []bool{false, true} {
		table := NewTaskTable()
		table.AddInputs(task.id, FingerprintBasis(FSPath(input)))
		if planned {
			table.PlanOperation("producer", input)
		}
		session := sessionOn(state)
		judgement, err := table.JudgeBasis(t.Context(), session.Store, task, RunPlans)
		if err != nil {
			t.Fatal(err)
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		if judgement.Current == planned {
			t.Errorf("planned change to the Basis %v: Current = %v", planned, judgement.Current)
		}
	}
}
