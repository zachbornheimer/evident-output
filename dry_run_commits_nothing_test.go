package evo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// runBasisTask runs one Task whose Basis is Value("v", value) against state
// and reports whether its Define ran.
func runBasisTask(t *testing.T, state, value string, dry bool) bool {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, StateDir: state, DryRun: dry})
	ran := false
	task := out.Task("basis-task")
	task.Basis(evo.Value("v", value))
	task.Define(func(context.Context) error { ran = true; return nil })
	if err := task.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	_ = out.Finish()
	_ = out.Close()
	return ran
}

// A dry run applies nothing, so it must not record the Basis it saw: the next
// real run still owes the change.
func TestDryRunNeverRecordsABasisItDidNotApply(t *testing.T) {
	state := t.TempDir()
	if !runBasisTask(t, state, "a", false) {
		t.Fatal("first real run must run Define")
	}
	if !runBasisTask(t, state, "b", true) {
		t.Fatal("a dry run with a changed Basis must still run Define")
	}
	if !runBasisTask(t, state, "b", false) {
		t.Fatal("real run after a dry run skipped the Task as current: the dry run recorded a Basis it never applied")
	}
}

// runTrackedExec runs one Task with a tracked Exec and reports whether the
// child spawned.
func runTrackedExec(t *testing.T, state, work string, dry bool) bool {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, StateDir: state, DryRun: dry})
	spawned := false
	task := out.Task("exec")
	task.Define(func(ctx context.Context) error {
		res, err := evo.Exec(ctx, evo.ExecSpec{
			Executable: "/bin/sh", Args: []string{"-c", "printf out > output.txt"}, Dir: work,
			Basis: []evo.Fingerprint{evo.FSPath(filepath.Join(work, "input.txt"))}, Outputs: []string{"output.txt"},
		})
		spawned = res.Ran
		return err
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	_ = out.Finish()
	_ = out.Close()
	return spawned
}

// A dry run of work that is already current must leave its records alone, so
// the next real run still skips it.
func TestDryRunLeavesCurrentOperationRecordsIntact(t *testing.T) {
	state, work := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "input.txt"), []byte("in"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !runTrackedExec(t, state, work, false) {
		t.Fatal("first real run must spawn the Exec")
	}
	if runTrackedExec(t, state, work, true) {
		t.Fatal("a dry run must never spawn the Exec")
	}
	if runTrackedExec(t, state, work, false) {
		t.Fatal("real run after a dry run re-spawned current work: the dry run wiped its operation record")
	}
}
