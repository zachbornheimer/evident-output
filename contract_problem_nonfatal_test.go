package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestProblemNeverEndsATask pins CONTRACT.md §4: Task.Problem records a
// finding and never ends the Task. A Define that records a Problem and
// returns nil is Succeeded with exit 0, and the Problem stays attached.
func TestProblemNeverEndsATask(t *testing.T) {
	const finding = "protected ref"
	const subject = "origin/main"
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "problem", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					task := r.Task("a")
					task.Define(func(context.Context) error {
						task.Problem(finding, evo.On(subject), evo.Code("protected"))
						return nil
					})
					return nil
				})
			if got := run.Result.Conclusion.Outcome; got != evo.Succeeded {
				t.Errorf("Conclusion.Outcome = %q, want %q", got, evo.Succeeded)
			}
			if got := run.Result.ExitCode(); got != 0 {
				t.Errorf("Result.ExitCode() = %d, want 0", got)
			}
			switch mode {
			case evo.JSON:
				assertProblemAttached(t, runDocument(t, run.Stdout), finding)
			case evo.JSONL:
			default:
				if !strings.Contains(run.Text(), finding) {
					t.Errorf("text does not show the Problem %q:\n%s", finding, run.Text())
				}
			}
		})
	}
}

// assertProblemAttached checks some Task in doc carries the Problem message.
func assertProblemAttached(t *testing.T, doc map[string]any, message string) {
	t.Helper()
	for _, task := range documentTasks(t, doc) {
		list, _ := task["problems"].([]any)
		for _, item := range list {
			if p, _ := item.(map[string]any); p != nil && p["message"] == message {
				return
			}
		}
	}
	t.Errorf("no task in evo.run carries the Problem %q", message)
}
