package evo

import (
	"bytes"
	"strings"
	"testing"
)

// Contract §13: "normal successful Task    hide ordinary Facts ...
// verbose    show Task/run Facts ... JSON/JSONL    retain structured Facts
// regardless of human verbosity", and "A generic Task-scoped or run-scoped
// Fact is not promoted merely because some Task failed". §21: "Normal
// successful Tasks do not show routine Facts by default" and "Unrelated
// configuration Facts stay hidden in normal mode even if the Task fails".

func renderFactTask(t *testing.T, verbosity verbosity, resolve func(*Task)) (string, *Run) {
	t.Helper()
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Verbose: verbosity == verbosityVerbose, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("worktrees")
	task.Fact("on disk", "508.8 MB")
	resolve(task)
	_ = out.finish()
	return buf.String(), out
}

func TestTaskFact_HiddenOnNormalSuccess(t *testing.T) {
	got, out := renderFactTask(t, verbosityNormal, func(task *Task) { succeed(task, "168 checked") })
	if strings.Contains(got, "on disk") || !strings.Contains(got, "✓ worktrees  168 checked") {
		t.Fatalf("a routine task Fact must be hidden at normal verbosity:\n%s", got)
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, "508.8 MB") {
		t.Fatalf("JSON must keep every Fact regardless of human verbosity:\n%s", doc)
	}
}

func TestTaskFact_ShownUnderVerbose(t *testing.T) {
	got, _ := renderFactTask(t, verbosityVerbose, func(task *Task) { succeed(task, "168 checked") })
	if !strings.Contains(got, "  on disk  508.8 MB") {
		t.Fatalf("verbose must show the task Fact:\n%s", got)
	}
}

func TestTaskFact_NotPromotedByFailure(t *testing.T) {
	got, _ := renderFactTask(t, verbosityNormal, func(task *Task) { task.fail("remove failed") })
	if strings.Contains(got, "on disk") || !strings.Contains(got, "remove failed") {
		t.Fatalf("a task-scoped Fact is not promoted merely because its Task failed:\n%s", got)
	}
}
