package evo_test

import (
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestOutputTask_SameNameGetsOrCreates pins L1: Output.Task (the instance
// method) must match evo.Task's get-or-create identity — repeated calls with
// the same name return the live handle instead of declaring a duplicate row.
// Before the fix, out.Task("gate.ready") called twice from two code paths
// declared two rows (the repo-retire P0: a second declare-new site under a
// name already in use produced a ledger with a duplicate row instead of one
// live handle).
func TestOutputTask_SameNameGetsOrCreates(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	first := out.Task("gate.ready")
	second := out.Task("gate.ready")

	if first.Snapshot().ID != second.Snapshot().ID {
		t.Fatalf("expected same handle identity, got %q and %q", first.Snapshot().ID, second.Snapshot().ID)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("expected no misuse error, got %v", err)
	}

	snap := out.Snapshot()
	if len(snap.Tasks) != 1 {
		t.Fatalf("expected exactly one declared task, got %d", len(snap.Tasks))
	}
}

// TestOutputTask_SameNameSameID_GetsOrCreates is the exact repo-retire P0
// shape: two call sites declare the same name under the same explicit
// evo.ID. That must resolve to the one live handle, not ErrDuplicateKey.
func TestOutputTask_SameNameSameID_GetsOrCreates(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	first := out.Task("ready", evo.ID("gate.ready"))
	second := out.Task("ready", evo.ID("gate.ready"))

	if first.Snapshot().ID != second.Snapshot().ID {
		t.Fatalf("expected same handle identity, got %q and %q", first.Snapshot().ID, second.Snapshot().ID)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("expected no misuse error, got %v", err)
	}
}

// TestOutputTask_DifferentNameSameID_StillDuplicateKey preserves the existing
// invariant: reusing one explicit evo.ID under two different names is a real
// identity conflict, not a get-or-create — ErrDuplicateKey must still fire.
func TestOutputTask_DifferentNameSameID_StillDuplicateKey(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("a", evo.ID("same"))
	out.Task("b", evo.ID("same"))

	if !errors.Is(out.Err(), evo.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got %v", out.Err())
	}
}
