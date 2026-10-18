package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestDefine_SubmitsAndNeverRunsInline pins CONTRACT.md §2: Define submits
// work and returns before the callback runs; a nil return succeeds.
func TestDefine_SubmitsAndNeverRunsInline(t *testing.T) {
	ranBeforeDefineReturned := false
	defineReturned := false
	run := executeContract(context.Background(), t, evo.Config{Name: "inline"}, func(_ context.Context, r *evo.Run) error {
		r.Task("work").Define(func(context.Context) error {
			ranBeforeDefineReturned = !defineReturned
			return nil
		})
		defineReturned = true
		return nil
	})
	if ranBeforeDefineReturned {
		t.Fatal("Define ran its callback inline, before returning")
	}
	if run.Result.ExitCode() != 0 {
		t.Fatalf("a nil-returning callback exited %d, want 0\n%s%s", run.Result.ExitCode(), run.Stdout, run.Stderr)
	}
}
