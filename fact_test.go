package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTaskFact_RendersInlineDimNoBang proves task.Fact renders inline on a
// Done task's own row, dim, with no "!" — the P8 contrast with
// TaskHandle.Warn's attention glyph (user-13-problems.md Problem 8: "Tasks
// are work. Facts are information.").
func TestTaskFact_RendersInlineDimNoBang(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	scan := out.Task("remote-tracking")
	scan.Fact("stale", "1")
	scan.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "stale  1") {
		t.Fatalf("want inline fact %q in:\n%s", "stale  1", got)
	}
	if strings.Contains(got, "!") {
		t.Fatalf("a Fact must never render the warning attention glyph:\n%s", got)
	}
}

// TestTaskFact_NeverResolvesTask proves Fact is a pure annotation: it does
// not resolve the task, so a subsequent Done still succeeds (mirrors
// TaskHandle.Warn's non-terminal contract).
func TestTaskFact_NeverResolvesTask(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.NoColor()}})
	task := out.Task("t")
	task.Fact("language", "go")
	task.Done()
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != evo.Done {
		t.Fatalf("Fact must not resolve the task, got %+v", snap.Tasks)
	}
	if len(snap.Tasks[0].Facts) != 1 || snap.Tasks[0].Facts[0].Value != "go" {
		t.Fatalf("want the recorded Fact on the snapshot, got %+v", snap.Tasks[0].Facts)
	}
}

// TestOutputWarn_FeedsWarnedModifierNotHeadline proves evo.Warn (run-scoped,
// P8 symmetry with TaskHandle.Warn) contributes to Conclusion.Warned/
// "· warned" without ever becoming a new headline state — a run with only a
// bare evo.Warn and no tasks still concludes StateReady, warned.
func TestOutputWarn_FeedsWarnedModifierNotHeadline(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Warn("no config file found, using defaults")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady (a bare Warn must not invent a headline)", c.State)
	}
	if !c.Warned {
		t.Fatal("want Conclusion.Warned = true after evo.Warn")
	}
	got := buf.String()
	if !strings.Contains(got, "no config file found, using defaults") {
		t.Fatalf("want the run-scoped warning text rendered, got:\n%s", got)
	}
	if !strings.Contains(got, "· warned") {
		t.Fatalf("want the \"· warned\" band, got:\n%s", got)
	}
}

// TestOutputFact_RendersDurableDimLine proves evo.Fact (run-scoped) renders
// as a fire-and-forget durable dim line, distinct from any task.
func TestOutputFact_RendersDurableDimLine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Fact("language", "go")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "language  go") {
		t.Fatalf("want run-scoped fact line, got:\n%s", got)
	}
}
