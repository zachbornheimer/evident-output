package freshness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// commitOperations stores ops as key's committed record in a fresh store.
func commitOperations(t *testing.T, key string, ops ...OperationRecord) *ManifestStore {
	t.Helper()
	store, err := OpenManifest(t.Context(), ManifestConfig{StateDir: t.TempDir()}, fakeEnvironment{})
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CommitTask(t.Context(), ApplicationRecord{ID: "app"}, TaskRecord{Key: key, Operations: ops}); err != nil {
		t.Fatalf("commit task: %v", err)
	}
	return store
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileConsultReportsEachWayAnOperationGoesStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed.txt")
	writeFile(t, path, "desired")
	basis := []BasisRecord{{Kind: "value", Key: "input", Digest: "one"}}
	call := FileCall{Path: path, ContentsManaged: true, Contents: []byte("desired"), Basis: basis}
	recorded, err := call.Record(t.Context())
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	store := commitOperations(t, "task", recorded)

	other := func(change func(*FileCall)) FileCall {
		changed := call
		change(&changed)
		return changed
	}
	cases := []struct {
		name    string
		call    FileCall
		key     string
		ordinal int
		current bool
		reason  string
	}{
		{"unchanged", call, "task", 0, true, ReasonCurrent},
		{"no record for the task", call, "other task", 0, false, ReasonNoPriorRecord},
		{"no record at the ordinal", call, "task", 1, false, ReasonNoPriorRecord},
		{"basis drift", other(func(c *FileCall) { c.Basis = []BasisRecord{{Kind: "value", Key: "input", Digest: "two"}} }), "task", 0, false, ReasonBasisDrift},
		{"definition drift", other(func(c *FileCall) { c.Contents = []byte("changed") }), "task", 0, false, ReasonDefinitionDrift},
	}
	for _, c := range cases {
		verdict, err := c.call.Consult(t.Context(), store, c.key, c.ordinal)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if verdict.Current != c.current || verdict.Reason != c.reason {
			t.Errorf("%s: current=%v reason=%q, want current=%v reason=%q", c.name, verdict.Current, verdict.Reason, c.current, c.reason)
		}
	}

	writeFile(t, path, "edited outside Evo")
	verdict, err := call.Consult(t.Context(), store, "task", 0)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Current || verdict.Reason != ReasonTrackedOutputDrift {
		t.Fatalf("edited output: current=%v reason=%q, want tracked_output_drift", verdict.Current, verdict.Reason)
	}
	if verdict.Prior.DefinitionFingerprint != recorded.DefinitionFingerprint {
		t.Fatal("a stale verdict must still carry the prior record")
	}
}

func TestExecEvaluateDecidesRunSkipOrPlan(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "tool")
	output := filepath.Join(dir, "out.txt")
	writeFile(t, executable, "#!/bin/sh\n")
	writeFile(t, output, "built")
	call := ExecCall{ExecutablePath: executable, Args: []string{"-v"}, Dir: dir, Outputs: []string{output}}

	empty := commitOperations(t, "task")
	noOutputs := ExecCall{ExecutablePath: executable, Dir: dir}
	eval, err := noOutputs.Evaluate(t.Context(), empty, "task", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.Spawns() || eval.Reason != ReasonNoOutputsDeclared {
		t.Fatalf("no declared outputs: spawns=%v reason=%q, want always-run", eval.Spawns(), eval.Reason)
	}

	stale, err := call.Evaluate(t.Context(), empty, "task", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Spawns() || stale.Reason != ReasonNoPriorRecord || stale.DefinitionFingerprint == "" {
		t.Fatalf("no prior record: %+v, want a spawn carrying the fresh definition", stale)
	}
	planned, err := call.Evaluate(t.Context(), empty, "task", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if planned.Disposition != ExecIsPlanned || planned.Spawns() {
		t.Fatalf("dry run of a stale call: %+v, want planned and not spawned", planned)
	}

	digest, err := PathOutputDigest(t.Context(), output)
	if err != nil {
		t.Fatal(err)
	}
	prior := OperationRecord{
		Kind:                  OperationKindExec,
		DefinitionFingerprint: stale.DefinitionFingerprint,
		Outputs:               []OutputRecord{{Kind: OutputKindExec, Path: output, Digest: digest}},
	}
	store := commitOperations(t, "task", prior)
	current, err := call.Evaluate(t.Context(), store, "task", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if current.Disposition != ExecIsCurrent || current.Reason != ReasonCurrent {
		t.Fatalf("matching prior record: %+v, want current even in a dry run", current)
	}

	writeFile(t, output, "tampered")
	drifted, err := call.Evaluate(t.Context(), store, "task", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !drifted.Spawns() || drifted.Reason != ReasonTrackedOutputDrift {
		t.Fatalf("edited output: %+v, want a spawn for tracked_output_drift", drifted)
	}
}

func TestRecordedOutputsHoldOnlyWhileTheyKeepTheirIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	writeFile(t, path, "built")
	digest, err := PathOutputDigest(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ops := []OperationRecord{{Outputs: []OutputRecord{{Path: path, Digest: digest}}}}
	if holds, err := RecordedOutputsHold(t.Context(), ops); err != nil || !holds {
		t.Fatalf("unchanged output: holds=%v err=%v, want holds", holds, err)
	}
	writeFile(t, path, "changed")
	if holds, err := RecordedOutputsHold(t.Context(), ops); err != nil || holds {
		t.Fatalf("changed output: holds=%v err=%v, want not holding without an error", holds, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := RecordedOutputsHold(ctx, ops); err == nil {
		t.Fatal("a done context must surface as an error, not as stale")
	}
}
