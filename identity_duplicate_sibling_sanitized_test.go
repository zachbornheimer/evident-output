package evo_test

import (
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestGroup_ControlCharacterVariants_AreDuplicateSiblings pins §3.1 for
// evo.Group: dedup and the stable key both derive from the same normalized
// name, so two raw names that sanitize to the same text ("deploy\x01" and
// "deploy\x02" both neutralize to "deploy") cannot bypass duplicate-sibling
// detection by differing only in a control character stripped before the
// key is computed.
func TestGroup_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	out.Group("deploy\x01")
	out.Group("deploy\x02")

	if err := out.Err(); !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}

// TestSequence_ControlCharacterVariants_AreDuplicateSiblings is Group's
// counterpart for evo.Sequence.
func TestSequence_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	out.Sequence("release\x01")
	out.Sequence("release\x02")

	if err := out.Err(); !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}

// TestNestedGroup_ControlCharacterVariants_AreDuplicateSiblings covers a
// container nested under another Group (GroupHandle.Group), which shares
// declareChildContainerLocked with GroupHandle.Sequence.
func TestNestedGroup_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	parent := out.Group("phase")
	parent.Group("build\x01")
	parent.Group("build\x02")

	if err := out.Err(); !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}

// TestGroupTask_ControlCharacterVariants_AreDuplicateSiblings covers a Task
// declared under a Group (GroupHandle.Task / declareGroupTask).
func TestGroupTask_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("phase")
	group.Task("migrate\x01")
	group.Task("migrate\x02")

	if err := out.Err(); !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}
