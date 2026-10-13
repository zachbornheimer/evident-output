package evo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExampleInit, ExampleRun, ExampleMain, ExampleTask, ExampleGroup, and
// ExampleSequence live in example_runtime_test.go / example_task_test.go /
// example_group_test.go — the stdlib-audit's per-topic split of what this
// file used to document inline.

// ExampleTask_Define submits a Task's atomic work to the scheduler —
// Define is the scheduling and execution boundary (spec §7): the callback
// runs on an eligible run unless a current pre-Define Verify already
// proved the desired state satisfied.
func ExampleTask_Define() {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, stderr: io.Discard, plain: true})
	task := out.Task("migrate")
	task.Define(func(ctx context.Context) error { return nil })
	_ = task.wait()
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ migrate
}

// ExampleTask_Verify registers a boolean, read-only pre-Define check.
// A Verify that reports the desired state already holds skips Define
// entirely and resolves ResolutionAlreadySatisfied — the one advanced
// escape hatch for domains Evo cannot track automatically (spec §56).
func ExampleTask_Verify() {
	out := initRun(Config{isolated: true, stdout: io.Discard, stderr: io.Discard})
	task := out.Task("already-configured")
	task.Verify(func(ctx context.Context) (bool, error) { return true, nil })
	task.Define(func(ctx context.Context) error {
		panic("Define must not run once Verify reports already satisfied")
	})
	_ = task.wait()
	fmt.Println(task.snapshot().State)
	// Output:
	// satisfied
}

// ExampleFile declares/reconciles one managed-state file resource: Evo
// creates it, rewrites it when Contents differ, and no-ops when the
// desired state already holds — common file work never needs a hand-
// written Evidence callback (spec §8, §56).
func ExampleFile() {
	dir, err := os.MkdirTemp("", "evo-example-file")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "config.json")

	out := initRun(Config{isolated: true, stateDir: dir, stdout: io.Discard, stderr: io.Discard})
	result := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		task := out.Task("config")
		task.Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: path, Contents: []byte(`{"ok":true}`)})
		})
		return nil
	})
	contents, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result.ExitCode())
	fmt.Println(string(contents))
	// Output:
	// 0
	// {"ok":true}
}

// ExampleTask_Fact records discovered information — not work — as a durable
// dim line attached to the Task that found it. Fact never resolves the
// Task and never fakes a checkmark merely to display a value.
func ExampleTask_Fact() {
	var buf bytes.Buffer
	// A Task Fact is routine context: it renders only under verbose (contract §13).
	out := initRun(Config{isolated: true, stdout: &buf, stderr: io.Discard, plain: true, verbosity: verbosityVerbose})
	scan := out.Task("remote-tracking")
	scan.Fact("stale", "1")
	scan.Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ remote-tracking    stale  1
}
