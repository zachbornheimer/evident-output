package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestExecWithoutOutputsIsNotAChange pins CONTRACT section 3: an Exec with no Outputs runs on every Run and
// is recorded as having run, never as having changed anything. Running read-only checks leaves
// Conclusion.Changed false and the band unchanged.
func TestExecWithoutOutputsIsNotAChange(t *testing.T) {
	w := newFreshnessWorld(t)
	for pass := 1; pass <= 2; pass++ {
		run := executeContract(context.Background(), t,
			evo.Config{Name: "check", Output: evo.Plain, State: evo.State{Dir: w.state, AppID: "contract"}},
			func(_ context.Context, r *evo.Run) error {
				r.Task("echo").Define(func(ctx context.Context) error {
					res, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"true"}})
					if err == nil && !res.Ran {
						t.Errorf("pass %d: Exec with no Outputs did not run", pass)
					}
					return err
				})
				return nil
			})
		if run.Result.Conclusion.Changed {
			t.Errorf("pass %d: Conclusion.Changed = true for an Exec that declared no Outputs:\n%s", pass, run.Text())
		}
		if strings.Contains(run.Text(), "[changed]") {
			t.Errorf("pass %d: the output reads [changed] for an Exec that declared no Outputs:\n%s", pass, run.Text())
		}
	}

	t.Run("the run document records that it ran", func(t *testing.T) {
		run := executeContract(context.Background(), t,
			evo.Config{Name: "check", Output: evo.JSON, State: evo.State{Dir: w.state, AppID: "contract"}},
			func(_ context.Context, r *evo.Run) error {
				r.Task("echo").Define(func(ctx context.Context) error {
					_, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"true"}})
					return err
				})
				return nil
			})
		data, _ := runDocument(t, run.Stdout)["data"].(map[string]any)
		task := firstTask(t, data)
		operations, _ := task["operations"].([]any)
		if len(operations) != 1 || operations[0].(map[string]any)["executed"] != true {
			t.Errorf("operations = %v, want one executed exec", task["operations"])
		}
		if effects, _ := data["effects"].([]any); len(effects) != 0 {
			t.Errorf("effects = %v, want none for an Exec that declared no Outputs", effects)
		}
	})
}

// firstTask is the one Task an evo.run document's data lists.
func firstTask(t *testing.T, data map[string]any) map[string]any {
	t.Helper()
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("data.tasks = %v, want one Task", data["tasks"])
	}
	return tasks[0].(map[string]any)
}
