package evo

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// ExampleTask_handle shows a Task carrying work via Define instead of a bare
// Done — the handle Task returns.
func ExampleTask_handle() {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: io.Discard, plain: true})
	handle := out.Task("fetch")
	handle.Define(func(ctx context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ fetch
}

// ExampleTask_Define_wait shows Define's fluent Wait sugar (ZYS-849
// Decisions): the common single-Task shape is `return task.Define(fn).Wait()`
// — Define still submits fn to the scheduler asynchronously, and Wait is
// what actually blocks for its outcome.
func ExampleTask_Define_wait() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	task := out.Task("fetch")
	err := task.Define(func(ctx context.Context) error { return nil }).wait()
	_ = out.finish()
	fmt.Println(err)
	// Output:
	// <nil>
}

// ExampleTaskSnapshot reads a Task's immutable view after it resolves.
func ExampleTaskSnapshot() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	task := out.Task("apply patch")
	task.Define(func(context.Context) error { return nil })
	_ = out.finish()
	snap := task.snapshot()
	fmt.Println(snap.Name, snap.State)
	// Output:
	// apply patch succeeded
}

// ExampleEvidencePhase shows one Verify observation attempt: whether it was
// evaluated, and whether the desired state was already satisfied.
func ExampleEvidencePhase() {
	phase := evidencePhase{Evaluated: true, Satisfied: true, Source: "verify"}
	fmt.Println(phase.Evaluated, phase.Satisfied, phase.Source)
	// Output:
	// true true verify
}

// ExampleTaskEvidence preserves both observation phases a Task's Verify may
// record: a before-false/after-true transition is never collapsed into one
// final boolean.
func ExampleTaskEvidence() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	task := out.Task("ensure directory")
	first := true
	task.Verify(func(ctx context.Context) (bool, error) {
		satisfied := !first
		first = false
		return satisfied, nil
	})
	task.Define(func(ctx context.Context) error { return nil })
	_ = out.finish()
	ev := task.snapshot().Evidence
	fmt.Println(ev.Before.Satisfied, ev.After.Satisfied)
	// Output:
	// false true
}
