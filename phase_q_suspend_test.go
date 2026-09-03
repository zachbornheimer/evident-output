package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestSuspend_NoPostResumeFrameWhenNothingRunning is red-first for evo-rec.md
// finding E: after a Suspend-wrapped child exits, the parent must not repaint
// a live frame when no task/item is Running. A Done task with determinate
// progress still counts as "live activity" for VisibilityDelay purposes, but
// that is not the same as something actively Running — resuming Suspend must
// not paint a stray frame over already-settled state.
func TestSuspend_NoPostResumeFrameWhenNothingRunning(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor(), testkit.Width(80))
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("scan")
	task.Progress(1, 1)
	task.Done()

	before := screen.LiveFrameCount()
	if err := out.Suspend(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	after := screen.LiveFrameCount()
	if after != before {
		t.Fatalf("Suspend repainted a live frame with nothing Running: before=%d after=%d ops=%+v", before, after, screen.Operations())
	}
}
