package evo

import (
	"errors"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/misuse"
)

// TestOutputTask_SameNameIsDuplicateSibling pins §3.1: Output.Task (the
// instance method) matches Task's identity contract — a repeated name
// under the same parent is a duplicate sibling declaration, not a
// get-or-create. 1.0 removed get-or-create because two distinct
// declarations silently merging into one identity is exactly the ambiguity
// manifest reconciliation cannot tolerate (a merged identity could later
// report a false "already satisfied").
func TestOutputTask_SameNameIsDuplicateSibling(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	first := out.Task("gate.ready")
	second := out.Task("gate.ready")

	if first.snapshot().ID == second.snapshot().ID {
		t.Fatal("expected a distinct handle for the duplicate declaration")
	}
	if err := out.err(); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}

	snap := out.snapshot()
	if len(snap.Tasks) != 2 {
		t.Fatalf("expected the original task plus one Failed duplicate-sibling row, got %d", len(snap.Tasks))
	}
}

// TestOutputTask_DifferentNameSameKey_StillDuplicateKey preserves the
// invariant: reusing one explicit Key under two different names is a real
// identity conflict, distinct from §3.1's duplicate sibling name —
// ErrDuplicateKey must still fire, not ErrDuplicateSiblingName.
func TestOutputTask_DifferentNameSameKey_StillDuplicateKey(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	out.Task("a").key("same")
	out.Task("b").key("same")

	if !errors.Is(out.err(), misuse.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got %v", out.err())
	}
}
