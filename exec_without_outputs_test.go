package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestExecWithoutOutputsIsNotAChange pins CONTRACT section 3: an Exec with no Outputs runs on every Run and
// is observed as having run, never as having changed anything. Running read-only checks leaves
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
		if !strings.Contains(run.Text(), "ran true") {
			t.Errorf("pass %d: the output does not say the Exec ran:\n%s", pass, run.Text())
		}
	}
}
