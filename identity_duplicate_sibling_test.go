package evo_test

import (
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestOutputTask_SameNameIsDuplicateSibling pins §3.1: Output.Task (the
// instance method) matches evo.Task's identity contract — a repeated name
// under the same parent is a duplicate sibling declaration, not a
// get-or-create. 1.0 removed get-or-create because two distinct
// declarations silently merging into one identity is exactly the ambiguity
// manifest reconciliation cannot tolerate (a merged identity could later
// report a false "already satisfied").
func TestOutputTask_SameNameIsDuplicateSibling(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	first := out.Task("gate.ready")
	second := out.Task("gate.ready")

	if first.Snapshot().ID == second.Snapshot().ID {
		t.Fatal("expected a distinct handle for the duplicate declaration")
	}
	if err := out.Err(); !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}

	snap := out.Snapshot()
	if len(snap.Tasks) != 2 {
		t.Fatalf("expected the original task plus one Failed duplicate-sibling row, got %d", len(snap.Tasks))
	}
}

// TestOutputTask_DifferentNameSameID_StillDuplicateKey preserves the
// pre-existing invariant: reusing one explicit evo.ID under two different
// names is a real identity conflict, distinct from §3.1's duplicate sibling
// name — ErrDuplicateKey must still fire, not ErrDuplicateSiblingName.
func TestOutputTask_DifferentNameSameID_StillDuplicateKey(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	out.TaskIdentified("a", "same")
	out.TaskIdentified("b", "same")

	if !errors.Is(out.Err(), evo.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got %v", out.Err())
	}
}
