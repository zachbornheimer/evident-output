package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestTaskFact_RendersInlineDimNoBang proves task.Fact renders inline on a
// Done task's own row, dim, with no "!" — the P8 contrast with
// Task.Warn's attention glyph (user-13-problems.md Problem 8: "Tasks
// are work. Facts are information.").
func TestTaskFact_RendersInlineDimNoBang(t *testing.T) {
	var buf bytes.Buffer
	// CONTRACT §8: a Task Fact hides until Verbose; this block is the verbose view.
	out := initRun(Config{Verbose: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	scan := out.Task("remote-tracking")
	scan.Fact("stale", "1")
	succeed(scan)

	if err := out.finish(); err != nil {
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
// Task.Warn's non-terminal contract).
func TestTaskFact_NeverResolvesTask(t *testing.T) {
	out := initRun(Config{}, tuning{color: colorNever})
	task := out.Task("t")
	task.Fact("language", "go")
	succeed(task)
	snap := out.snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != succeeded {
		t.Fatalf("Fact must not resolve the task, got %+v", snap.Tasks)
	}
	if len(snap.Tasks[0].Facts) != 1 || snap.Tasks[0].Facts[0].Value != "go" {
		t.Fatalf("want the recorded Fact on the snapshot, got %+v", snap.Tasks[0].Facts)
	}
}

// TestOutputFact_RendersDurableDimLine proves Fact (run-scoped) renders
// as a fire-and-forget durable dim line, distinct from any task.
func TestOutputFact_RendersDurableDimLine(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	out.Fact("language", "go")

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "language  go") {
		t.Fatalf("want run-scoped fact line, got:\n%s", got)
	}
}
