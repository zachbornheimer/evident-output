package evo

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/misuse"
)

// TestTaskHandle_KeyBeforeDefineOverridesDefaultIdentity proves §3.1's
// advanced override: Key called before Define replaces the default
// kind+parent-key+name derivation, and the override is visible on the
// snapshot.
func TestTaskHandle_KeyBeforeDefineOverridesDefaultIdentity(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("write plist")
	before := task.snapshot().Key
	task.key("launch-agent.write-plist")
	after := task.snapshot().Key

	if after != "launch-agent.write-plist" {
		t.Fatalf("Key() = %q, want the explicit override", after)
	}
	if before == after {
		t.Fatalf("default key %q was already the override — the test proves nothing", before)
	}
	succeed(task)
	if err := out.err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

// TestTaskHandle_KeyAfterDefineIsAnError proves §7's freeze contract: Define
// freezes dependency/verification/execution configuration, and identity
// (§3.1) is part of that configuration — Key after Define is refused and
// leaves the task's key untouched.
func TestTaskHandle_KeyAfterDefineIsAnError(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("write plist")
	task.Define(func(context.Context) error { return nil })
	before := task.snapshot().Key
	task.key("too-late")
	after := task.snapshot().Key

	if after != before {
		t.Fatalf("Key() after Define changed the key: before=%q after=%q", before, after)
	}
	if !errors.Is(out.err(), misuse.ErrKeyAfterDefine) {
		t.Fatalf("Err() = %v, want ErrKeyAfterDefine", out.err())
	}
}

// TestTaskHandle_DuplicateExplicitKeyOfSameKindIsRejected proves §3.1:
// "Duplicate explicit keys of the same entity kind are an error" — two
// Tasks racing for the same explicit key is a real identity conflict, not
// a duplicate-sibling-name (different mechanism, same underlying "identity
// must be unambiguous" property).
func TestTaskHandle_DuplicateExplicitKeyOfSameKindIsRejected(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	first := out.Task("write plist")
	first.key("launch-agent.write-plist")

	second := out.Task("write plist copy")
	second.key("launch-agent.write-plist")

	if !errors.Is(out.err(), misuse.ErrDuplicateKey) {
		t.Fatalf("Err() = %v, want ErrDuplicateKey", out.err())
	}
	if second.snapshot().Key == first.snapshot().Key {
		t.Fatal("the second Task must not have claimed the already-taken key")
	}
}

// TestTaskHandle_RepeatingTheSameKeyIsIdempotent: setting a Task's key to
// the key it already has is not a conflict with itself.
func TestTaskHandle_RepeatingTheSameKeyIsIdempotent(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("write plist").key("x").key("x")
	if got := task.snapshot().Key; got != "x" {
		t.Fatalf("Key() = %q, want x", got)
	}
	if err := out.err(); err != nil {
		t.Fatalf("Err() = %v, want nil for an idempotent Key", err)
	}
}

// TestTaskHandle_KeyAfterATerminalVerbIsAnError: a settled row's identity
// is frozen like a Defined one's; Key after Fail must not rewrite it.
func TestTaskHandle_KeyAfterATerminalVerbIsAnError(t *testing.T) {
	out := initRun(Config{stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("write plist")
	task.fail("could not write")
	before := task.snapshot().Key
	task.key("late")
	if after := task.snapshot().Key; after != before {
		t.Fatalf("Key() after Fail changed the key: before=%q after=%q", before, after)
	}
	if !errors.Is(out.err(), misuse.ErrKeyAfterDefine) {
		t.Fatalf("Err() = %v, want ErrKeyAfterDefine", out.err())
	}
}
