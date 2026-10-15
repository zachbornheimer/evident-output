package evo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// overturnWorld is the two Tasks of a same-output conflict and the state
// directory that remembers them: "first" settles Done before "second", which
// ran after it, claims the same path and overturns it.
type overturnWorld struct {
	shared string
	state  string
}

func newOverturnWorld(t *testing.T) overturnWorld {
	t.Helper()
	return overturnWorld{shared: filepath.Join(t.TempDir(), "shared.txt"), state: t.TempDir()}
}

func (w overturnWorld) config(mode evo.Output) evo.Config {
	return evo.Config{Name: "overturn", Output: mode, State: evo.State{Dir: w.state, AppID: "contract"}}
}

// conflictingRun writes w.shared from "first" and then from "second".
func (w overturnWorld) conflictingRun(ctx context.Context, t *testing.T, mode evo.Output) contractRun {
	t.Helper()
	return executeContract(ctx, t, w.config(mode), func(_ context.Context, r *evo.Run) error {
		var previous *evo.Task
		for _, name := range []string{"first", "second"} {
			task := r.Task(name)
			if previous != nil {
				task.After(previous)
			}
			task.Define(func(ctx context.Context) error {
				return evo.File(ctx, evo.FileSpec{Path: w.shared, Contents: []byte(name)})
			})
			previous = task
		}
		return nil
	})
}

// laterRunOfFirst is the next Run: "first" alone, writing what it wrote
// before, with JSONL on so the test can see how the operation was judged.
func (w overturnWorld) laterRunOfFirst(ctx context.Context, t *testing.T) contractRun {
	t.Helper()
	return executeContract(ctx, t, w.config(evo.JSONL), func(_ context.Context, r *evo.Run) error {
		r.Task("first").Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: w.shared, Contents: []byte("first")})
		})
		return nil
	})
}

// finishedOutcomes is the outcome of each task.finished event named task
// carries, in order.
func finishedOutcomes(events []map[string]any, taskID string) []any {
	var outcomes []any
	for _, event := range eventsOfType(events, "task.finished") {
		if event["entity_id"] == taskID {
			outcomes = append(outcomes, payload(event)["outcome"])
		}
	}
	return outcomes
}

// declaredTaskID is the entity id of the Task named name in a JSONL run.
func declaredTaskID(t *testing.T, events []map[string]any, name string) string {
	t.Helper()
	for _, event := range eventsOfType(events, "task.declared") {
		if payload(event)["name"] == name {
			id, _ := event["entity_id"].(string)
			return id
		}
	}
	t.Fatalf("no task.declared event for %q", name)
	return ""
}

// TestOverturnedTaskIsConsistentEverywhere pins that a Task failed after it
// settled Done (the first writer of a same-output conflict) reads Failed in
// every projection, with the earlier success still in the history, and that
// the manifest does not remember it as done.
func TestOverturnedTaskIsConsistentEverywhere(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			ctx := context.Background()
			w := newOverturnWorld(t)
			run := w.conflictingRun(ctx, t, mode)

			if got := run.Result.Conclusion.Outcome; got != evo.Failed {
				t.Errorf("Conclusion.Outcome = %v, want failed", got)
			}
			if code := run.Result.ExitCode(); code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			switch mode {
			case evo.JSON:
				assertOverturnedInDocument(t, runDocument(t, run.Stdout))
			case evo.JSONL:
				assertOverturnedInEvents(t, runEvents(t, run.Stdout))
			default:
				assertOverturnedInStream(t, run.Text())
			}

			later := w.laterRunOfFirst(ctx, t)
			events := runEvents(t, later.Stdout)
			if skipped := eventsOfType(events, "operation.skipped_current"); len(skipped) > 0 {
				t.Errorf("a later Run skipped the overturned Task's File as current (%d events); the manifest still remembers it as done", len(skipped))
			}
		})
	}
}

// assertOverturnedInStream checks the text history: the success the Task
// first showed, then its failure.
func assertOverturnedInStream(t *testing.T, text string) {
	t.Helper()
	succeeded, failed := strings.Index(text, "✓ first"), strings.LastIndex(text, "✗ first")
	if succeeded < 0 || failed < 0 || failed < succeeded {
		t.Errorf("stream does not show first succeeding and then failing (✓ at %d, ✗ at %d):\n%s", succeeded, failed, text)
	}
}

// assertOverturnedInDocument checks the evo.run document reads the Task and
// the run Failed.
func assertOverturnedInDocument(t *testing.T, doc map[string]any) {
	t.Helper()
	if doc["outcome"] != "failed" {
		t.Errorf("evo.run outcome = %v, want failed", doc["outcome"])
	}
	for _, task := range documentTasks(t, doc) {
		if task["name"] == "first" && task["outcome"] != "failed" {
			t.Errorf("evo.run Task first outcome = %v, want failed", task["outcome"])
		}
	}
}

// assertOverturnedInEvents checks the event stream tells the Task finished
// succeeded and then finished failed.
func assertOverturnedInEvents(t *testing.T, events []map[string]any) {
	t.Helper()
	got := finishedOutcomes(events, declaredTaskID(t, events, "first"))
	if len(got) != 2 || got[0] != "succeeded" || got[1] != "failed" {
		t.Errorf("task.finished outcomes for first = %v, want [succeeded failed]", got)
	}
}
