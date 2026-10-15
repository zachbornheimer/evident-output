package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestStopperIsTheTaskNotTheSummaryText pins CONTRACT.md §2: a dependent names
// the Task that stopped it, never text a caller wrote. A Task nobody Defined
// whose Summary reads like a stopper's still stops its dependent as itself.
func TestStopperIsTheTaskNotTheSummaryText(t *testing.T) {
	run := executeContract(context.Background(), t, evo.Config{Name: "stopper", Output: evo.JSON},
		func(_ context.Context, r *evo.Run) error {
			lookalike := r.Task("lookalike").Summary("not started — elsewhere failed")
			r.Task("after").After(lookalike).Define(func(context.Context) error { return nil })
			return nil
		})
	for _, task := range documentTasks(t, runDocument(t, run.Stdout)) {
		if task["name"] != "after" {
			continue
		}
		if got, want := task["summary"], "not started — lookalike not started"; got != want {
			t.Errorf("dependent summary = %q, want %q: the stopper is the Task, not its Summary text", got, want)
		}
	}
}
