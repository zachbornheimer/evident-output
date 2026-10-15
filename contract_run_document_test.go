package evo_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// measuredTask is how long the timed Task works, so its timing is
// unmistakably non-zero.
const measuredTask = 30 * time.Millisecond

// TestRunDocumentStatesOnlyTrueFacts pins CONTRACT.md §8: evo.run reports
// only what happened. A Define that ran and failed executed its definition;
// timing, when present, is measured; a Task that wrote a File lists the
// operation and the basis it read.
func TestRunDocumentStatesOnlyTrueFacts(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	run := executeContract(context.Background(), t,
		evo.Config{Name: "true facts", Output: evo.JSON, State: evo.State{Dir: t.TempDir(), AppID: "contract"}},
		func(_ context.Context, r *evo.Run) error {
			r.Task("broken").Define(func(context.Context) error { return errors.New("boom") })
			r.Task("timed").Define(func(context.Context) error {
				time.Sleep(measuredTask)
				return nil
			})
			r.Task("writer").Define(func(ctx context.Context) error {
				if err := evo.File(ctx, evo.FileSpec{Path: input, Contents: []byte("input\n")}); err != nil {
					return err
				}
				return evo.File(ctx, evo.FileSpec{
					Path: filepath.Join(dir, "out.txt"), Contents: []byte("out\n"), Inputs: []string{input},
				})
			})
			return nil
		})
	tasks := map[string]map[string]any{}
	for _, task := range documentTasks(t, runDocument(t, run.Stdout)) {
		name, _ := task["name"].(string)
		tasks[name] = task
	}

	if got := tasks["broken"]["definition_executed"]; got != true {
		t.Errorf("broken: definition_executed = %v, want true for a Define that ran and failed", got)
	}
	if timing, present := tasks["timed"]["timing"].(map[string]any); present {
		for _, key := range []string{"running_ms", "total_ms"} {
			if ms, _ := timing[key].(float64); ms < float64(measuredTask.Milliseconds()) {
				t.Errorf("timed: timing.%s = %v, want at least %d for a %s Task", key, timing[key], measuredTask.Milliseconds(), measuredTask)
			}
		}
	}
	for _, key := range []string{"operations", "basis"} {
		if list, _ := tasks["writer"][key].([]any); len(list) == 0 {
			t.Errorf("writer: %s = %v, want what the Task's File calls did and read", key, tasks["writer"][key])
		}
	}
}
