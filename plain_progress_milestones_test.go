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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	task := out.Task("download")
	for i := 0; i <= 100; i += 10 {
		task.Progress(i, 100)
	}
	task.Done()
	_ = out.Finish()

	rendered := buf.String()
	if strings.Count(rendered, "download") < 3 {
		t.Fatalf("expected multiple progress lines (milestone-thinned), got only:\n%s", rendered)
	}
	if !strings.Contains(rendered, "100/100") {
		t.Fatalf("expected a final n/n line, got:\n%s", rendered)
	}
}

// TestPlainProgress_NoSpinnerGlyph is beginner-8: a plain-mode Running row
// never shows a spinner-alphabet frame — there is no animation loop behind
// a durable, one-shot-per-milestone line, so a frozen mid-spin frame is
// misleading, not informative.
func TestPlainProgress_NoSpinnerGlyph(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("download").Progress(0, 100)

	rendered := buf.String()
	for _, spinnerFrame := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		if strings.Contains(rendered, spinnerFrame) {
			t.Fatalf("plain output contains a spinner-alphabet frame %q:\n%s", spinnerFrame, rendered)
		}
	}
}
