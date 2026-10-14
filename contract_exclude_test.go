package evo_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
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

// TestAfterGroupProceedsWhenAMemberIsExcluded pins CONTRACT.md §2: After a
// container waits on the container's own outcome, and an Excluded member
// folds into its success, so the dependent runs and the run exits 0.
func TestAfterGroupProceedsWhenAMemberIsExcluded(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			var pushed atomic.Bool
			run := executeContract(context.Background(), t, evo.Config{Name: "excluded", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					items := r.Group("items")
					for _, name := range []string{"a", "b", "c"} {
						items.Task(name).Define(func(context.Context) error {
							if name == "b" {
								return evo.Exclude("protected")
							}
							return nil
						})
					}
					r.Task("push").After(items).Define(func(context.Context) error {
						pushed.Store(true)
						return nil
					})
					return nil
				})
			if !pushed.Load() {
				t.Fatalf("push ordered After a Group with an Excluded member never ran:\n%s", run.Text())
			}
			if code := run.Result.ExitCode(); code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
		})
	}
}

// TestNotStartedNamesItsPredecessorWithOneSpelling pins CONTRACT.md §2 and
// §4: a Task an Excluded predecessor kept from running names that
// predecessor in every projection, and NotStarted carries no outcome in
// evo.run or evo.event alike.
func TestNotStartedNamesItsPredecessorWithOneSpelling(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "excluded", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					gate := r.Task("protected gate")
					gate.Define(func(context.Context) error { return evo.Exclude("protected") })
					r.Task("dependent").After(gate).Define(func(context.Context) error { return nil })
					return nil
				})
			switch mode {
			case evo.JSON:
				for _, task := range documentTasks(t, runDocument(t, run.Stdout)) {
					if task["name"] != "dependent" {
						continue
					}
					if _, has := task["outcome"]; has {
						t.Errorf("evo.run NotStarted task carries outcome %v, want none", task["outcome"])
					}
					if summary, _ := task["summary"].(string); !strings.Contains(summary, "protected gate") {
						t.Errorf("evo.run NotStarted summary = %q, want it to name protected gate", summary)
					}
				}
			case evo.JSONL:
				finished := eventsOfType(runEvents(t, run.Stdout), "task.finished")
				for _, event := range finished {
					p := payload(event)
					if p["summary"] == nil || !strings.Contains(p["summary"].(string), "not started") {
						continue
					}
					if _, has := p["outcome"]; has {
						t.Errorf("task.finished for a NotStarted task carries outcome %v, want none", p["outcome"])
					}
					if !strings.Contains(p["summary"].(string), "protected gate") {
						t.Errorf("task.finished summary = %q, want it to name protected gate", p["summary"])
					}
				}
			default:
				if !rowNames(run.Text(), "dependent", "protected gate") {
					t.Errorf("the NotStarted row does not name protected gate:\n%s", run.Text())
				}
			}
		})
	}
}

// rowNames reports whether some line naming task also names cause.
func rowNames(text, task, cause string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, task) && strings.Contains(line, cause) {
			return true
		}
	}
	return false
}
