package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	unsatisfiedVerifyExplanation = "postcondition not satisfied"
	unsatisfiedVerifyProblemCode = "verification-unsatisfied"
	unsatisfiedVerifyTaskName    = "converge"
)

// TestVerifyFalseAfterSuccessfulDefineFails pins CONTRACT.md §2: a Verify
// that is false after a successful callback fails the Task. The Task settles
// failed, its Problem carries the verification-unsatisfied code, and the run
// exits 2 in every Output mode.
func TestVerifyFalseAfterSuccessfulDefineFails(t *testing.T) {
	for _, output := range contractOutputs {
		t.Run(outputName[output], func(t *testing.T) {
			defineRan := false
			run := executeContract(context.Background(), t, evo.Config{Name: "verify", Output: output}, func(_ context.Context, r *evo.Run) error {
				r.Task(unsatisfiedVerifyTaskName).
					Verify(func(context.Context) (bool, error) { return false, nil }).
					Define(func(context.Context) error {
						defineRan = true
						return nil
					})
				return nil
			})
			if !defineRan {
				t.Fatal("Define never ran, so Verify was not judged after a successful callback")
			}
			if run.Result.ExitCode() != 2 {
				t.Fatalf("exit = %d, want 2\n%s", run.Result.ExitCode(), run.Text())
			}
			assertVerifyFailureReported(t, output, run)
		})
	}
}

// assertVerifyFailureReported checks the failed Task and its Problem in the
// shape the Output mode writes.
func assertVerifyFailureReported(t *testing.T, output evo.Output, run contractRun) {
	t.Helper()
	switch output {
	case evo.JSON:
		tasks := documentTasks(t, runDocument(t, run.Stdout))
		if len(tasks) != 1 || tasks[0]["state"] != "failed" {
			t.Fatalf("tasks = %v, want one failed Task", tasks)
		}
		problems, _ := tasks[0]["problems"].([]any)
		if len(problems) != 1 || problems[0].(map[string]any)["code"] != unsatisfiedVerifyProblemCode {
			t.Fatalf("problems = %v, want one %s", problems, unsatisfiedVerifyProblemCode)
		}
	case evo.JSONL:
		finished := eventsOfType(runEvents(t, run.Stdout), "task.finished")
		if len(finished) != 1 || payload(finished[0])["outcome"] != "failed" {
			t.Fatalf("task.finished events = %v, want one failed", finished)
		}
		if payload(finished[0])["summary"] != unsatisfiedVerifyExplanation {
			t.Fatalf("task.finished summary = %v, want %q", payload(finished[0])["summary"], unsatisfiedVerifyExplanation)
		}
	default:
		text := run.Text()
		if !strings.Contains(text, unsatisfiedVerifyTaskName) || !strings.Contains(text, unsatisfiedVerifyExplanation) {
			t.Fatalf("output does not name the failed Task and %q:\n%s", unsatisfiedVerifyExplanation, text)
		}
	}
}
