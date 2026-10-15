package evo

import (
	"context"
	"io"
	"testing"
)

type operationShape struct {
	kind     string
	executed bool
	current  bool
}

// operationsOf is the operations the finished run's evo.run document lists
// for the Task named name.
func operationsOf(t *testing.T, out *Run, name string) []operationShape {
	t.Helper()
	var listed []operationShape
	for _, task := range out.decodedRunDocument(t).Data.Tasks {
		if task.Name != name {
			continue
		}
		for _, op := range task.Operations {
			listed = append(listed, operationShape{op.Kind, op.Executed, op.Current})
		}
	}
	return listed
}

// TestFailedFileRecordsItsAttemptedOperation pins CONTRACT.md §8: a Task's
// operations list what its File calls did, and a File whose chmod failed did
// run: it lists an executed operation that is not current, never an empty
// list beside a verification that shows the error.
func TestFailedFileRecordsItsAttemptedOperation(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	spec, fsys := newFailedChmodFile(t, dir, "hook.sh")
	out := initRun(Config{Output: JSON, State: State{Dir: t.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, FS: fsys}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	out.Task("install hook").Define(func(ctx context.Context) error { return File(ctx, spec) })
	_ = out.finish()

	got, want := operationsOf(t, out, "install hook"), []operationShape{{"file", true, false}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("install hook operations = %v, want %v", got, want)
	}
}

// TestFailedExecRecordsItsAttemptedOperation is the same promise for Exec: a
// child that exited nonzero ran, so it is listed as executed and not current.
func TestFailedExecRecordsItsAttemptedOperation(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	out := initRun(Config{Output: JSON, State: State{Dir: t.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Process: &scriptedRunner{exitCode: 1}}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	_ = runExecTask(t, out, "build", ExecSpec{executable: tool, Dir: dir})
	_ = out.finish()

	got, want := operationsOf(t, out, "build"), []operationShape{{"exec", true, false}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("build operations = %v, want %v", got, want)
	}
}
