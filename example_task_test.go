package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleTask declares a Task on the default instance and resolves it.
func ExampleTask() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	evo.Task("working tree").Done()
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ working tree
}

// ExampleTaskHandle shows a Task carrying work via Define instead of a bare
// Done — the handle Task returns.
func ExampleTaskHandle() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true, Isolated: true})
	handle := out.Task("fetch")
	handle.Define(func(ctx context.Context) error { return nil })
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ fetch
}

// ExampleTaskSnapshot reads a Task's immutable view after it resolves.
func ExampleTaskSnapshot() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	task := out.Task("apply patch")
	task.Done()
	_ = out.Finish()
	snap := task.Snapshot()
	fmt.Println(snap.Name, snap.State)
	// Output:
	// apply patch done
}

// ExampleEvidencePhase shows one Verify observation attempt: whether it was
// evaluated, and whether the desired state was already satisfied.
func ExampleEvidencePhase() {
	phase := evo.EvidencePhase{Evaluated: true, Satisfied: true, Source: "verify"}
	fmt.Println(phase.Evaluated, phase.Satisfied, phase.Source)
	// Output:
	// true true verify
}

// ExampleTaskEvidence preserves both observation phases a Task's Verify may
// record: a before-false/after-true transition is never collapsed into one
// final boolean.
func ExampleTaskEvidence() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	task := out.Task("ensure directory")
	first := true
	task.Verify(func(ctx context.Context) (bool, error) {
		satisfied := !first
		first = false
		return satisfied, nil
	})
	task.Define(func(ctx context.Context) error { return nil })
	_ = out.Finish()
	ev := task.Snapshot().Evidence
	fmt.Println(ev.Before.Satisfied, ev.After.Satisfied)
	// Output:
	// false true
}
