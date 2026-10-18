package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// taskSummaries maps each Task in an evo.run document to its summary.
func taskSummaries(t *testing.T, stdout string) map[string]string {
	t.Helper()
	summaries := map[string]string{}
	for _, task := range documentTasks(t, runDocument(t, stdout)) {
		summaries[task["name"].(string)], _ = task["summary"].(string)
	}
	return summaries
}

// TestNotStartedNamesPredecessorRegardlessOfBuild pins CONTRACT.md §2: a
// NotStarted Task's summary names the Task at the root of its chain of
// predecessors, in that Task's state, whether the Group it ran After got its
// members from Build, declared them directly, or the chain is plain Tasks.
func TestNotStartedNamesPredecessorRegardlessOfBuild(t *testing.T) {
	ok := func(context.Context) error { return nil }
	for _, ending := range stopperEndings {
		want := "not started — gate " + ending.state
		for _, built := range []bool{true, false} {
			name := "static/" + ending.name
			if built {
				name = "built/" + ending.name
			}
			t.Run(name, func(t *testing.T) {
				got := taskSummaries(t, stopperRun(t, built, ending, evo.JSON).Stdout)["report"]
				if got != want {
					t.Errorf("report summary = %q, want %q", got, want)
				}
			})
		}
		t.Run("chain/"+ending.name, func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "chain", Output: evo.JSON},
				func(_ context.Context, r *evo.Run) error {
					gate := r.Task("gate")
					middle := r.Task("middle").After(gate)
					r.Task("last").After(middle).Define(ok)
					middle.Define(ok)
					gate.Define(func(context.Context) error { return ending.verb() })
					return nil
				})
			for name, got := range taskSummaries(t, run.Stdout) {
				if name != "gate" && got != want {
					t.Errorf("%s summary = %q, want %q", name, got, want)
				}
			}
		})
	}
}
