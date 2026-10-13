package evo_test

import (
	"context"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestExcludedPredecessorLeavesDependentsNotStarted pins CONTRACT.md §2: a
// Task that returns Exclude leaves the Tasks ordered After it NotStarted, so
// their callbacks never run, in every Output mode.
func TestExcludedPredecessorLeavesDependentsNotStarted(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			var dependentRan atomic.Bool
			run := executeContract(context.Background(), t, evo.Config{Name: "excluded", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					excluded := r.Task("excluded")
					excluded.Define(func(context.Context) error { return evo.Exclude("protected") })
					r.Task("dependent").After(excluded).Define(func(context.Context) error {
						dependentRan.Store(true)
						return nil
					})
					return nil
				})
			if dependentRan.Load() {
				t.Error("a Task ordered After an Excluded Task ran; want it left NotStarted")
			}
			if mode == evo.JSON {
				assertDependentNotSucceeded(t, runDocument(t, run.Stdout))
			}
		})
	}
}

// assertDependentNotSucceeded checks the document does not claim the
// dependent ran to success.
func assertDependentNotSucceeded(t *testing.T, doc map[string]any) {
	t.Helper()
	for _, task := range documentTasks(t, doc) {
		if task["name"] == "dependent" && task["outcome"] == string(evo.Succeeded) {
			t.Errorf("evo.run task dependent outcome = %v, want it not Succeeded", task["outcome"])
		}
	}
}
