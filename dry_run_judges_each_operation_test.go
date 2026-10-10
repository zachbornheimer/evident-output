package evo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// runTaskOf runs one Task whose Define calls each operation in order, against
// state, and returns the JSONL event stream.
func runTaskOf(t *testing.T, state string, dry bool, operations ...func(context.Context) error) string {
	t.Helper()
	var stream syncBuffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: state, Format: evo.FormatJSONL, Stdout: &stream, Stderr: &stream, DryRun: dry})
	task := out.Task("two-operations").Define(func(ctx context.Context) error {
		for _, operation := range operations {
			if err := operation(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	_ = out.Finish()
	if err := out.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	return stream.String()
}

func writeFile(path, contents string) func(context.Context) error {
	return func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte(contents), Mode: 0o644})
	}
}

func execWriting(dir, output string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := evo.Exec(ctx, evo.ExecSpec{
			Executable: "/bin/sh", Args: []string{"-c", "printf x > " + output}, Dir: dir, Outputs: []string{output},
		})
		return err
	}
}

// currentOperations is how many operations the stream skipped as current.
func currentOperations(stream string) int { return strings.Count(stream, `"reason":"current"`) }

// A dry run of a Task with several tracked operations must judge each against
// that operation's own record, as the real run would: after a real run that
// left everything current, a dry run plans nothing.
func TestDryRunJudgesEveryFileOfATaskAgainstItsOwnRecord(t *testing.T) {
	state, work := t.TempDir(), t.TempDir()
	one, two := writeFile(filepath.Join(work, "one.txt"), "1"), writeFile(filepath.Join(work, "two.txt"), "2")
	runTaskOf(t, state, false, one, two)

	real := runTaskOf(t, state, false, one, two)
	dry := runTaskOf(t, state, true, one, two)

	if currentOperations(real) != 2 {
		t.Fatalf("second real run: %d operations current, want 2\n%s", currentOperations(real), real)
	}
	if currentOperations(dry) != 2 {
		t.Fatalf("dry run found %d operations current, the real run 2: it planned a change nothing would make\n%s", currentOperations(dry), dry)
	}
}

// A stale operation the dry run only plans still takes its place in the Task's
// order: the operation after it is judged against its own record.
func TestDryRunJudgesAFileAfterAPlannedOneAgainstItsOwnRecord(t *testing.T) {
	state, work := t.TempDir(), t.TempDir()
	one, two := filepath.Join(work, "one.txt"), filepath.Join(work, "two.txt")
	runTaskOf(t, state, false, writeFile(one, "1"), writeFile(two, "2"))

	dry := runTaskOf(t, state, true, writeFile(one, "changed"), writeFile(two, "2"))

	if currentOperations(dry) != 1 {
		t.Fatalf("dry run found %d operations current, want 1 (the second File)\n%s", currentOperations(dry), dry)
	}
}

// The same holds for Exec, which keeps its own record per operation.
func TestDryRunJudgesEveryExecOfATaskAgainstItsOwnRecord(t *testing.T) {
	state, work := t.TempDir(), t.TempDir()
	first, second := execWriting(work, "a.txt"), execWriting(work, "b.txt")
	runTaskOf(t, state, false, first, second)

	real := runTaskOf(t, state, false, first, second)
	dry := runTaskOf(t, state, true, first, second)

	if currentOperations(real) != 2 {
		t.Fatalf("second real run: %d operations current, want 2\n%s", currentOperations(real), real)
	}
	if currentOperations(dry) != 2 {
		t.Fatalf("dry run found %d operations current, the real run 2\n%s", currentOperations(dry), dry)
	}
}
