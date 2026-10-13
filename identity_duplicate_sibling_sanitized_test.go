package evo

import (
	"errors"
	"io"
	"testing"
)

// TestGroup_ControlCharacterVariants_AreDuplicateSiblings pins §3.1 for
// Group: dedup and the stable key both derive from the same normalized
// name, so two raw names that sanitize to the same text ("deploy\x01" and
// "deploy\x02" both neutralize to "deploy") cannot bypass duplicate-sibling
// detection by differing only in a control character stripped before the
// key is computed.
func TestGroup_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	out.Group("deploy\x01")
	out.Group("deploy\x02")

	if err := out.err(); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}

// TestSequence_ControlCharacterVariants_AreDuplicateSiblings is Group's
// counterpart for Sequence.
func TestSequence_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	out.Sequence("release\x01")
	out.Sequence("release\x02")

	if err := out.err(); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}

// TestNestedGroup_ControlCharacterVariants_AreDuplicateSiblings covers a
// container nested under another Group (Group.Group), which shares
// declareChildContainerLocked with Group.Sequence.
func TestNestedGroup_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	parent := out.Group("phase")
	parent.Group("build\x01")
	parent.Group("build\x02")

	if err := out.err(); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}

// TestGroupTask_ControlCharacterVariants_AreDuplicateSiblings covers a Task
// declared under a Group (Group.Task / declareGroupTask).
func TestGroupTask_ControlCharacterVariants_AreDuplicateSiblings(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("phase")
	group.Task("migrate\x01")
	group.Task("migrate\x02")

	if err := out.err(); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("expected ErrDuplicateSiblingName, got %v", err)
	}
}
