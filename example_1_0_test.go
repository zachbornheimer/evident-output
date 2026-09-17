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

// ExampleInit constructs the package-default Output — the sole
// constructor (spec §1.1). Call it as the first statement in main, before
// any other I/O, so first paint can arm.
func ExampleInit() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "ghost", Stdout: &buf, Stderr: io.Discard, Plain: true})
	out.Task("working tree").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ working tree
	//
	// [ready]  ghost
}

// ExampleRun executes run against the default instance and returns the
// finished Result (Conclusion plus the application error) without exiting
// the process — the shape a caller composing its own exit path uses.
func ExampleRun() {
	var buf bytes.Buffer
	evo.Init(evo.Config{Title: "ghost", Stdout: &buf, Stderr: io.Discard, Plain: true})
	result := evo.Run(context.Background(), func(ctx context.Context) error {
		evo.Task("working tree").Done()
		return nil
	})
	fmt.Print(buf.String())
	fmt.Println(result.ExitCode())
	// Output:
	// ✓ working tree
	//
	// [ready]  ghost
	// 0
}

// ExampleMain derives the process exit code from run's outcome and returns
// it; it never calls os.Exit itself, so os.Exit(evo.Main(run)) is the
// ordinary main() (spec §1.1's CLI shape).
func ExampleMain() {
	var buf bytes.Buffer
	evo.Init(evo.Config{Title: "ghost", Stdout: &buf, Stderr: io.Discard, Plain: true})
	code := evo.Main(func(ctx context.Context) error {
		evo.Task("working tree").Done()
		return nil
	})
	fmt.Print(buf.String())
	fmt.Println(code)
	// Output:
	// ✓ working tree
	//
	// [ready]  ghost
	// 0
}

// ExampleTask resolves one atomic unit directly — a check/gate with no
// callback, rendered as a fact row the instant it resolves.
func ExampleTask() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true})
	out.Task("working tree").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ working tree
}

// ExampleGroup declares an independent collection: one named child Task
// per item: the scheduler may overlap eligible children, and the Group's
// own state is derived from them — never resolved directly.
func ExampleGroup() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true})
	installs := out.Group("install")
	for _, pkg := range []string{"repo-a", "repo-b"} {
		installs.Task(pkg).Done()
	}
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ install
	//    ✓ repo-a
	//    ✓ repo-b
}

// ExampleSequence declares an ordered dependency of named children: a
// failed child auto-resolves later siblings to NotStarted instead of
// running them.
func ExampleSequence() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true})
	worktrees := out.Sequence("worktrees")
	for _, path := range []string{"repo-a", "repo-b"} {
		task := worktrees.Task(path)
		task.Define(func(ctx context.Context) error { return nil })
	}
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ worktrees
	//    ✓ repo-a
	//    ✓ repo-b
}

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

// ExampleFact records discovered information — not work — as a durable
// dim line attached to the Task that found it. Fact never resolves the
// Task and never fakes a checkmark merely to display a value.
func ExampleFact() {
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
