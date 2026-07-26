package evo_test

import (
	"bytes"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestOUT021_DataProjectionOption(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.New(
		evo.To(&primary),
		evo.Diagnostics(&diag),
		evo.DataProjection(),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })
	out.Task("scan").Phase("walk").Donef("ok")
	_ = out.Finish()
	// Data projection still renders human to primary in v0.3 path unless diagnostic set for UI;
	// ensure option is accepted and Finish works.
	if primary.Len() == 0 && diag.Len() == 0 {
		t.Fatal("expected some output")
	}
}

func TestAPI016_ExternalProjectionSnapshots(t *testing.T) {
	out, err := evo.NewWithConfig(evo.Config{
		Projection: evo.ProjectionExternal,
		Primary:    io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.Close() })
	ch := out.Snapshots()
	out.Item("x").OK()
	_ = out.Finish()
	got := false
	for range ch {
		got = true
	}
	if !got {
		t.Fatal("expected snapshots")
	}
}
