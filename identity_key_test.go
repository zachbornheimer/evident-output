package evo_test

import (
	"context"
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTaskHandle_KeyBeforeDefineOverridesDefaultIdentity proves §3.1's
// advanced override: Key called before Define replaces the default
// kind+parent-key+name derivation, and the override is visible on the
// snapshot.
func TestTaskHandle_KeyBeforeDefineOverridesDefaultIdentity(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("write plist")
	before := task.Snapshot().Key
	task.Key("launch-agent.write-plist")
	after := task.Snapshot().Key

	if after != "launch-agent.write-plist" {
		t.Fatalf("Key() = %q, want the explicit override", after)
	}
	if before == after {
		t.Fatalf("default key %q was already the override — the test proves nothing", before)
	}
	task.Done()
	if err := out.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

// TestTaskHandle_KeyAfterDefineIsAnError proves §7's freeze contract: Define
// freezes dependency/verification/execution configuration, and identity
// (§3.1) is part of that configuration — Key after Define is refused and
// leaves the task's key untouched.
func TestTaskHandle_KeyAfterDefineIsAnError(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("write plist")
	task.Define(func(context.Context) error { return nil })
	before := task.Snapshot().Key
	task.Key("too-late")
	after := task.Snapshot().Key

	if after != before {
		t.Fatalf("Key() after Define changed the key: before=%q after=%q", before, after)
	}
	if !errors.Is(out.Err(), evo.ErrKeyAfterDefine) {
		t.Fatalf("Err() = %v, want ErrKeyAfterDefine", out.Err())
	}
}

// TestTaskHandle_DuplicateExplicitKeyOfSameKindIsRejected proves §3.1:
// "Duplicate explicit keys of the same entity kind are an error" — two
// Tasks racing for the same explicit key is a real identity conflict, not
// a duplicate-sibling-name (different mechanism, same underlying "identity
// must be unambiguous" property).
func TestTaskHandle_DuplicateExplicitKeyOfSameKindIsRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	first := out.Task("write plist")
	first.Key("launch-agent.write-plist")

	second := out.Task("write plist copy")
	second.Key("launch-agent.write-plist")

	if !errors.Is(out.Err(), evo.ErrDuplicateKey) {
		t.Fatalf("Err() = %v, want ErrDuplicateKey", out.Err())
	}
	if second.Snapshot().Key == first.Snapshot().Key {
		t.Fatal("the second Task must not have claimed the already-taken key")
	}
}
