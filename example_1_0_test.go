package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleInit, ExampleRun, ExampleMain, ExampleTask, ExampleGroup, and
// ExampleSequence live in example_runtime_test.go / example_task_test.go /
// example_group_test.go — the stdlib-audit's per-topic split of what this
// file used to document inline.

// ExampleTaskHandle_Define submits a Task's atomic work to the scheduler —
// Define is the scheduling and execution boundary (spec §7): the callback
// runs on an eligible run unless a current pre-Define Verify already
// proved the desired state satisfied.
func ExampleTaskHandle_Define() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true})
	task := out.Task("migrate")
	task.Define(func(ctx context.Context) error { return nil })
	_ = task.Wait()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ migrate
}

// ExampleTaskHandle_Verify registers a boolean, read-only pre-Define check.
// A Verify that reports the desired state already holds skips Define
// entirely and resolves ResolutionAlreadySatisfied — the one advanced
// escape hatch for domains Evo cannot track automatically (spec §56).
func ExampleTaskHandle_Verify() {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	task := out.Task("already-configured")
	task.Verify(func(ctx context.Context) (bool, error) { return true, nil })
	task.Define(func(ctx context.Context) error {
		panic("Define must not run once Verify reports already satisfied")
	})
	_ = task.Wait()
	fmt.Println(task.Snapshot().Resolution)
	// Output:
	// already_satisfied
}

// ExampleTaskHandle_Key sets an advanced, refactor/rename-stable override
// for a Task's stable identity (spec §3.1) — use it when a Task's name
// changes across runs but its tracked identity must not.
func ExampleTaskHandle_Key() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true})
	out.Task("migrate 003_add_users.sql").Key("migration:003").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ migrate 003_add_users.sql
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

	out := evo.Init(evo.Config{Isolated: true, StateDir: dir, Stdout: io.Discard, Stderr: io.Discard})
	result := out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("config")
		task.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte(`{"ok":true}`)})
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
func ExampleTaskHandle_Fact() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true})
	scan := out.Task("remote-tracking")
	scan.Fact("stale", "1")
	scan.Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ remote-tracking    stale  1
}
