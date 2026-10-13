package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Task.Summary is non-terminal result metadata (1.1/ZYS-971
// Decisions 2026-09-23b): a Define callback that calls Summary and returns
// nil still resolves through Define's own outcome, and the Summary text
// renders on the successful terminal row exactly like Group.Summary does
// one level up.
func TestTask_SummaryRendersOnTheSuccessfulTerminalRow(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{title: "zq", isolated: true, plain: true, stdout: &buf, stderr: &buf})
	branches := out.Task("check branches")
	branches.Define(func(context.Context) error {
		branches.Summary("459 checked")
		return nil
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	transcript := buf.String()
	if !strings.Contains(transcript, "check branches") || !strings.Contains(transcript, "459 checked") {
		t.Fatalf("Summary text is gone from the successful terminal row:\n%s", transcript)
	}
}

// Summary never resolves the task on its own — only Define's own outcome
// (or another terminal verb) does. Calling Summary and nothing else must
// leave the task unresolved (auto-resolved Done only at Finish, per the
// same contract Warn documents).
func TestTask_SummaryDoesNotResolveTheTask(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{title: "zq", isolated: true, plain: true, stdout: &buf, stderr: &buf})
	task := out.Task("scan")
	task.Summary("not done yet")
	snap := task.snapshot()
	if snap.State != pending && snap.State != notStarted {
		t.Fatalf("Summary alone must not resolve the task; got state %v", snap.State)
	}
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_ = out.close()
}

// Last call wins; an empty call clears the field.
func TestTask_SummaryLastCallWinsAndEmptyClears(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{title: "zq", isolated: true, plain: true, stdout: &buf, stderr: &buf})
	task := out.Task("scan")
	task.Define(func(context.Context) error {
		task.Summary("first")
		task.Summary("second")
		return nil
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_ = out.close()
	if got := task.snapshot().Summary; got != "second" {
		t.Fatalf("last Summary call must win, got %q", got)
	}

	var buf2 bytes.Buffer
	out2 := initRun(Config{title: "zq", isolated: true, plain: true, stdout: &buf2, stderr: &buf2})
	task2 := out2.Task("scan")
	task2.Define(func(context.Context) error {
		task2.Summary("set then cleared")
		task2.Summary("")
		return nil
	})
	if err := out2.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_ = out2.close()
	if got := task2.snapshot().Summary; got != "" {
		t.Fatalf("empty Summary call must clear the field, got %q", got)
	}
}

// Summary survives TaskSnapshot and the JSON/JSONL wire shape as the same
// "summary" field Group.Summary projects.
func TestTask_SummarySurvivesJSONSnapshot(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{title: "zq", isolated: true, plain: true, stdout: &buf, stderr: &buf})
	task := out.Task("scan")
	task.Define(func(context.Context) error {
		task.Summary("4 stale refs")
		return nil
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	snap := out.snapshot()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if !strings.Contains(string(raw), `"Summary":"4 stale refs"`) {
		t.Fatalf("JSON snapshot is missing the task summary field:\n%s", raw)
	}
	_ = out.close()
}

// Summary must not paper over Running: while a task is still active,
// Doing/Progress/Step/Bytes own the live row, not the result Summary.
func TestTask_SummaryDoesNotReplaceDoingWhileRunning(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{title: "zq", isolated: true, plain: true, stdout: &buf, stderr: &buf})
	task := out.Task("scan")
	task.Doing("scanning refs")
	task.Summary("would-be result")
	snap := task.snapshot()
	if snap.Phase != "scanning refs" {
		t.Fatalf("Summary must not overwrite the live Doing phase, got phase %q", snap.Phase)
	}
	succeed(task)
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_ = out.close()
}
