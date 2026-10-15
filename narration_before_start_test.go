package evo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestNarrationBeforeStartDoesNotMarkRunning pins CONTRACT.md §4: only the
// scheduler starting a Task's callback makes it Running. Progress, Doing and
// Writer text a Task carries before that is held and shown once it starts, so
// a Task that is waiting or never runs shows no running row, no running time,
// and no task.started event ahead of its task.eligible.
func TestNarrationBeforeStartDoesNotMarkRunning(t *testing.T) {
	narrate := func(x *evo.Task) {
		x.Progress(0, 10)
		x.Doing("warming up")
		_, _ = x.Writer().Write([]byte("early output\n"))
	}
	waiting := func(_ context.Context, r *evo.Run) error {
		slow := r.Task("slow").Define(func(context.Context) error { time.Sleep(150 * time.Millisecond); return nil })
		x := r.Task("x").After(slow)
		narrate(x)
		x.Define(func(context.Context) error { return nil })
		return nil
	}
	neverRun := func(_ context.Context, r *evo.Run) error {
		narrate(r.Task("never"))
		return nil
	}

	t.Run("a waiting Task shows no running row in Plain", func(t *testing.T) {
		run := executeContract(context.Background(), t, evo.Config{Name: "narration", Output: evo.Plain}, waiting)
		beforeSlow, _, _ := strings.Cut(run.Stderr, "slow")
		if strings.Contains(beforeSlow, "◐") {
			t.Errorf("Plain shows a running row for a Task still waiting on slow:\n%s", run.Stderr)
		}
	})
	t.Run("task.eligible precedes task.started", func(t *testing.T) {
		run := executeContract(context.Background(), t, evo.Config{Name: "narration", Output: evo.JSONL}, waiting)
		events := runEvents(t, run.Stdout)
		eligible, started := eventIndex(events, "task.eligible", "x"), eventIndex(events, "task.started", "x")
		if eligible < 0 || started < 0 || eligible > started {
			t.Errorf("task.eligible at %d, task.started at %d for x; want eligible first", eligible, started)
		}
	})
	t.Run("a Task that never runs records no running time and no running row", func(t *testing.T) {
		plain := executeContract(context.Background(), t, evo.Config{Name: "narration", Output: evo.Plain}, neverRun)
		if strings.Contains(plain.Stderr, "◐") {
			t.Errorf("Plain shows a running row for a Task that never ran:\n%s", plain.Stderr)
		}
		doc := runDocument(t, executeContract(context.Background(), t, evo.Config{Name: "narration", Output: evo.JSON}, neverRun).Stdout)
		for _, task := range documentTasks(t, doc) {
			timing, _ := task["timing"].(map[string]any)
			if got := timing["running_ms"]; got != float64(0) {
				t.Errorf("running_ms = %v for a NotStarted Task, want 0", got)
			}
			if task["state"] != "not_started" {
				t.Errorf("state = %v, want not_started", task["state"])
			}
		}
	})
}

// eventIndex is the position of the first event of type typ about the Task
// named name, or -1.
func eventIndex(events []map[string]any, typ, name string) int {
	ids := map[string]string{}
	for _, event := range eventsOfType(events, "task.declared") {
		if id, _ := event["entity_id"].(string); id != "" {
			ids[id], _ = payload(event)["name"].(string)
		}
	}
	for i, event := range events {
		if id, _ := event["entity_id"].(string); event["type"] == typ && ids[id] == name {
			return i
		}
	}
	return -1
}
