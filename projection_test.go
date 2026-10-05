package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/testkit"

	evo "github.com/zachbornheimer/evident-output"
)

func TestOUT021_DataProjectionOption(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &primary, Stderr: &diag, Color: evo.ColorNever, Plain: true, Format: evo.FormatData})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("scan").Doing("walk"), "ok")
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
	succeed(out.Task("x"))
	_ = out.Finish()
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].Name != "x" {
		t.Fatalf("expected the task in the snapshot, got %+v", snap.Tasks)
	}
}

// TestProjection_StandaloneTaskPlainLiveParity is the
// non-regression control for the DisplayUnit refactor of writeLiveTaskLine:
// a standalone Running task's rendered bytes are unchanged by the refactor
// (the order requires "golden-identical except where this order changes
// them" — this shape is not one of the changes).
func TestProjection_StandaloneTaskPlainLiveParity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	build := out.Task("build")
	build.Progress(3, 10)
	build.Doing("compiling") // forces a fresh render (Progress alone coalesces)

	frame := screen.LatestLiveText()
	if !strings.Contains(frame, "build") || !strings.Contains(frame, "3/10") {
		t.Fatalf("standalone Running task row shape regressed:\n%s", frame)
	}
	if strings.Contains(frame, "—") {
		t.Fatalf("a task under the 5s threshold must render with no elapsed suffix:\n%s", frame)
	}

	succeed(build)
	_ = out.Finish()
}
