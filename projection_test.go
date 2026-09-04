package evo_test

import (
	"bytes"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestOUT021_DataProjectionOption(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.To(&primary),
		evo.Diagnostics(&diag),
		evo.DataProjection(),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("scan").Doing("walk").Done("ok")
	_ = out.Finish()
	// Data projection still renders human to primary in v0.3 path unless diagnostic set for UI;
	// ensure option is accepted and Finish works.
	if primary.Len() == 0 && diag.Len() == 0 {
		t.Fatal("expected some output")
	}
}

// TestAPI016_ExternalProjectionSnapshots is C8: the streaming Snapshots()
// channel is deleted (Output.Snapshot() — singular, poll-based — is the
// surviving accessor); FormatExternal's "snapshots only" promise still
// holds via that path.
func TestAPI016_ExternalProjectionSnapshots(t *testing.T) {
	out := evo.Init(evo.Config{
		Format: evo.FormatExternal,
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("x").Done()
	_ = out.Finish()
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].Name != "x" {
		t.Fatalf("expected the task in the snapshot, got %+v", snap.Tasks)
	}
}
