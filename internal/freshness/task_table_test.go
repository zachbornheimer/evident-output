package freshness

import (
	"context"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// testTask is the smallest TaskView.
type testTask struct {
	id  record.TaskID
	key string
}

func (t testTask) TaskID() record.TaskID { return t.id }
func (t testTask) ManifestKey() string   { return t.key }

func sessionOn(stateDir string) *ManifestSession {
	return NewManifestSession(func() ManifestConfig {
		return ManifestConfig{AppID: "app", StateDir: stateDir, Workspace: stateDir}
	})
}

func TestJudgeBasisOfATaskWithNoInputsIsUndeclared(t *testing.T) {
	judgement, err := NewTaskTable().JudgeBasis(t.Context(), sessionOn(t.TempDir()), testTask{id: "t1", key: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if judgement.Declared || judgement.Current {
		t.Fatalf("judgement = %+v, want undeclared and not current", judgement)
	}
}

// A Task is current only when a prior successful Run recorded the same Basis:
// the first Run executes, an identical second Run is current, and a changed
// input runs again.
func TestJudgeBasisIsCurrentOnlyAfterARunRecordedTheSameBasis(t *testing.T) {
	state := t.TempDir()
	task := testTask{id: "t1", key: "k"}

	firstSession, firstTable := sessionOn(state), NewTaskTable()
	firstTable.AddInputs(task.id, FingerprintBasis(Value("input", 1)), nil)
	first, err := firstTable.JudgeBasis(t.Context(), firstSession, task)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Declared || first.Current || len(first.Observed) != 1 {
		t.Fatalf("first run = %+v, want declared, not current, one record", first)
	}
	if commit := firstTable.Commit(t.Context(), firstSession, task); !commit.Written || commit.Err != nil || commit.Operations != 1 {
		t.Fatalf("first commit = %+v, want the Basis record written", commit)
	}
	if err := firstSession.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	sameSession, sameTable := sessionOn(state), NewTaskTable()
	sameTable.AddInputs(task.id, FingerprintBasis(Value("input", 1)))
	same, err := sameTable.JudgeBasis(t.Context(), sameSession, task)
	if err != nil {
		t.Fatal(err)
	}
	if !same.Current {
		t.Fatalf("second run with the same Basis = %+v, want current", same)
	}
	if err := sameSession.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	driftSession, driftTable := sessionOn(state), NewTaskTable()
	driftTable.AddInputs(task.id, FingerprintBasis(Value("input", 2)))
	drift, err := driftTable.JudgeBasis(t.Context(), driftSession, task)
	if err != nil {
		t.Fatal(err)
	}
	if drift.Current {
		t.Fatalf("second run with a changed input = %+v, want not current", drift)
	}
	if err := driftSession.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestJudgeBasisRejectsADuplicateInput(t *testing.T) {
	table := NewTaskTable()
	task := testTask{id: "t1", key: "k"}
	table.AddInputs(task.id, FingerprintBasis(Value("same", 1)), FingerprintBasis(Value("same", 2)))
	if _, err := table.JudgeBasis(t.Context(), sessionOn(t.TempDir()), task); err == nil {
		t.Fatal("duplicate (kind, key) inputs must be a programmer error")
	}
}

func TestOperationsAccumulateInCallOrderPerTask(t *testing.T) {
	table := NewTaskTable()
	table.AppendOperation("a", OperationRecord{Kind: OperationKindFile})
	table.AppendOperation("a", OperationRecord{Kind: OperationKindExec})
	table.AppendOperation("b", OperationRecord{Kind: OperationKindFile})
	if got := table.OperationCount("a"); got != 2 {
		t.Fatalf("count(a) = %d, want 2", got)
	}
	ops := table.Operations("a")
	if len(ops) != 2 || ops[0].Kind != OperationKindFile || ops[1].Kind != OperationKindExec {
		t.Fatalf("ops(a) = %+v, want file then exec", ops)
	}
	ops[0].Kind = "mutated"
	if table.Operations("a")[0].Kind != OperationKindFile {
		t.Fatal("Operations must return a copy")
	}
	if table.OperationCount("unknown") != 0 || table.Operations("unknown") != nil {
		t.Fatal("an unknown Task has no operations")
	}
}

// A Run that never opened a manifest commits nothing, so a purely opaque
// consumer's Run leaves no manifest file.
func TestCommitWritesNothingWhenNoManifestWasOpened(t *testing.T) {
	state := t.TempDir()
	table := NewTaskTable()
	table.AppendOperation("t1", OperationRecord{Kind: OperationKindFile})
	commit := table.Commit(t.Context(), sessionOn(state), testTask{id: "t1", key: "k"})
	if commit != (TaskCommit{}) {
		t.Fatalf("commit = %+v, want nothing", commit)
	}
}

// An opaque Task (no tracked operation, no Basis) is staged with the
// application-fingerprint fallback and written by the next Save.
func TestCommitStagesAnOpaqueTaskWithItsFallbackDefinition(t *testing.T) {
	state := t.TempDir()
	session := sessionOn(state)
	if _, err := session.Store(t.Context()); err != nil {
		t.Fatal(err)
	}
	table := NewTaskTable()
	task := testTask{id: "t1", key: "opaque"}
	if commit := table.Commit(t.Context(), session, task); commit != (TaskCommit{}) {
		t.Fatalf("commit = %+v, want staged, not written", commit)
	}
	store := session.Opened()
	if err := store.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	stored, ok := store.Task("opaque")
	if !ok || stored.DefinitionFingerprint != OpaqueTaskDefinitionFingerprint("opaque", session.Application().Fingerprint) {
		t.Fatalf("stored = %+v ok=%v, want the opaque fallback definition", stored, ok)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifiersKeepRegistrationOrderPerTask(t *testing.T) {
	table := NewTaskTable()
	var order []string
	table.AddVerifier("a", func(context.Context) (bool, error) { order = append(order, "first"); return true, nil })
	table.AddVerifier("a", func(context.Context) (bool, error) { order = append(order, "second"); return true, nil })
	if _, err := EvaluateVerifiers(t.Context(), table.Verifiers("a"), nil); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("order = %v, want registration order", order)
	}
	if len(table.Verifiers("b")) != 0 {
		t.Fatal("another Task has no verifiers")
	}
}
