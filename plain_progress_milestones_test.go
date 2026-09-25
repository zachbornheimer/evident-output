package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPlainProgress_StreamsMilestones_NotOnlyFirstTick is beginner-8: a
// large-total Running task must stream more than one durable progress line
// in plain mode — the old behavior streamed "progress established" once and
// then went silent until Done, which reads as a stalled/hung task in CI
// logs for anything that takes a while.
func TestPlainProgress_StreamsMilestones_NotOnlyFirstTick(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	task := out.Task("download")
	for i := 0; i <= 100; i += 10 {
		task.Progress(i, 100)
	}
	succeed(task)
	_ = out.Finish()

	rendered := buf.String()
	if strings.Count(rendered, "download") < 3 {
		t.Fatalf("expected multiple progress lines (milestone-thinned), got only:\n%s", rendered)
	}
	if !strings.Contains(rendered, "100/100") {
		t.Fatalf("expected a final n/n line, got:\n%s", rendered)
	}
}

// TestPlainProgress_DoingBeforeProgress_FinalMilestoneNotDropped is
// beginner-8's "always a final n/n" for the Doing-before-Progress loop
// order (task.Doing(item); work; task.Progress(i, n)) — the E-119 review's
// blocker: once the task's first Doing establishes namesItems, every
// milestone defers to the next Doing to name it, and the loop's very last
// milestone (total/total) has no Doing left to claim it, so it must be
// flushed at task resolution instead of silently dropped.
func TestPlainProgress_DoingBeforeProgress_FinalMilestoneNotDropped(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("sync")

	const total = 40
	for i := 1; i <= total; i++ {
		task.Doing("widget-%02d", i)
		task.Progress(i, total)
	}
	succeed(task, "synced")
	_ = out.Close()

	rendered := buf.String()
	if !strings.Contains(rendered, "40/40") {
		t.Fatalf("expected the final 40/40 milestone to stream, got:\n%s", rendered)
	}
}

// TestPlainProgress_NoSpinnerGlyph is beginner-8: a plain-mode Running row
// never shows a spinner-alphabet frame — there is no animation loop behind
// a durable, one-shot-per-milestone line, so a frozen mid-spin frame is
// misleading, not informative.
func TestPlainProgress_NoSpinnerGlyph(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	out.Task("download").Progress(0, 100)

	rendered := buf.String()
	for _, spinnerFrame := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		if strings.Contains(rendered, spinnerFrame) {
			t.Fatalf("plain output contains a spinner-alphabet frame %q:\n%s", spinnerFrame, rendered)
		}
	}
}
