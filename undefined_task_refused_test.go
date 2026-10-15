package evo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestUndefinedTaskIsRefusedDeclaration pins CONTRACT.md §7: a Task still
// undefined at Finish is a refused declaration, the same programmer error as
// Define(nil). The Task ends NotStarted (§4), the run fails by misuse with exit
// 2, and Result.Err wraps ErrInvalidConfig naming the Task, so a CI caller sees
// what a reader of the rows sees.
func TestUndefinedTaskIsRefusedDeclaration(t *testing.T) {
	run := executeContract(context.Background(), t, evo.Config{Name: "undefined", Output: evo.Plain},
		func(_ context.Context, r *evo.Run) error {
			deploy := r.Task("deploy")
			r.Task("after").After(deploy).Define(func(context.Context) error { return nil })
			return nil
		})

	if got, want := run.Result.ExitCode(), 2; got != want {
		t.Errorf("exit code = %d, want %d: a refused declaration fails the run", got, want)
	}
	if !errors.Is(run.Result.Err, evo.ErrInvalidConfig) {
		t.Fatalf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", run.Result.Err)
	}
	if !strings.Contains(run.Result.Err.Error(), `"deploy"`) {
		t.Errorf("Result.Err = %q, want it to name the undefined Task %q", run.Result.Err, "deploy")
	}
	if !strings.Contains(run.Result.Conclusion.Explanation, `"deploy"`) {
		t.Errorf("Conclusion.Explanation = %q, want it to name the undefined Task", run.Result.Conclusion.Explanation)
	}
	if !strings.Contains(run.Stderr, "- after  not started — deploy not started") {
		t.Errorf("the dependent must still name the undefined Task as its stopper:\n%s", run.Stderr)
	}
	if !strings.Contains(run.Stderr, "[failed · partial]  undefined") {
		t.Errorf("Plain band is not [failed · partial]:\n%s", run.Stderr)
	}
}
