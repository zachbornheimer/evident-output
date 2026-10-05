package evo_test

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestSequence_BlockStopsLaterSibling drives the shipped Sequence path:
// a task that Blocks inside Define must leave the next sibling NotStarted
// and must not run that sibling's callback.
func TestSequence_BlockStopsLaterSibling(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		Plain:    true,
		Color:    evo.ColorNever,
		Stdout:   &buf,
	})
	t.Cleanup(func() { _ = out.Close() })

	seq := out.Sequence("gate")
	gate := seq.Task("check")
	laterRan := false
	later := seq.Task("apply")
	gate.Define(func(context.Context) error {
		gate.Block("plan not executable")
		return nil
	})
	later.Define(func(context.Context) error {
		laterRan = true
		return nil
	})

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if laterRan {
		t.Fatal("apply callback ran after check blocked")
	}
	if got := later.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("apply state = %v, want NotStarted\n%s", got, buf.String())
	}
	if got := gate.Snapshot().State; got != evo.Blocked {
		t.Fatalf("check state = %v, want Blocked", got)
	}
}
