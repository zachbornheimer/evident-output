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
	if !strings.Contains(run.Stderr, "after   not started — deploy not started") {
		t.Errorf("the dependent must still name the undefined Task as its stopper:\n%s", run.Stderr)
	}
	if !strings.Contains(run.Stderr, "[failed · partial]  undefined") {
		t.Errorf("Plain band is not [failed · partial]:\n%s", run.Stderr)
	}
}

// TestUndefinedTaskIsRefusedBesideAFailure pins that a Task which failed does
// not excuse the omission: the run still exits 2, Result.Err still wraps
// ErrInvalidConfig, and Plain still prints the misuse line and the fix.
func TestUndefinedTaskIsRefusedBesideAFailure(t *testing.T) {
	run := executeContract(context.Background(), t, evo.Config{Name: "undefined", Output: evo.Plain},
		func(_ context.Context, r *evo.Run) error {
			r.Task("lint").Define(func(context.Context) error { return evo.Fail("lint failed") })
			r.Task("deploy")
			return nil
		})

	if got, want := run.Result.ExitCode(), 2; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	if !errors.Is(run.Result.Err, evo.ErrInvalidConfig) {
		t.Errorf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", run.Result.Err)
	}
	for _, want := range []string{`!  Task "deploy" has no Define callback`, `→  give Task "deploy" a Define callback`} {
		if !strings.Contains(run.Stderr, want) {
			t.Errorf("Plain output lacks %q:\n%s", want, run.Stderr)
		}
	}
}

// TestUndefinedTaskKeepsItsSummary pins that the refusal replaces no text the
// caller wrote: a Task that said something before Finish still says it in
// every projection, and its outcome alone reads NotStarted.
func TestUndefinedTaskKeepsItsSummary(t *testing.T) {
	body := func(_ context.Context, r *evo.Run) error {
		said := r.Task("said").Summary("said something")
		r.Task("after").After(said).Define(func(context.Context) error { return nil })
		return nil
	}
	plain := executeContract(context.Background(), t, evo.Config{Name: "undefined", Output: evo.Plain}, body)
	if !strings.Contains(plain.Stderr, "said something") {
		t.Errorf("Plain lost the Task's own Summary:\n%s", plain.Stderr)
	}
	for _, task := range documentTasks(t, runDocument(t, executeContract(context.Background(), t, evo.Config{Name: "undefined", Output: evo.JSON}, body).Stdout)) {
		switch task["name"] {
		case "said":
			if got := task["summary"]; got != "said something" {
				t.Errorf("evo.run summary = %v, want the Task's own %q", got, "said something")
			}
			if got := task["state"]; got != "not_started" {
				t.Errorf("evo.run state = %v, want not_started", got)
			}
		case "after":
			if got := task["summary"]; got != "not started — said not started" {
				t.Errorf("dependent summary = %v, want it to name the undefined Task", got)
			}
		}
	}
}
