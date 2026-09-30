package outcomes_test

import (
	"context"
	"fmt"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestC02_010_ProblemOptionsRecordStructuredParts(t *testing.T) {
	_, _, task := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error {
			task.Problem("bad config",
				evo.Detail("line is malformed"), evo.Code("E_CFG"), evo.On("app.yaml"),
				evo.Location("app.yaml", 7, 3), evo.Count(4, "keys"))
			return nil
		})
	})
	if len(task.Problems) != 1 {
		t.Fatalf("problems = %+v", task.Problems)
	}
	p := task.Problems[0]
	if p.Detail != "line is malformed" || p.Code != "E_CFG" || p.Subject != "app.yaml" {
		t.Fatalf("problem = %+v", p)
	}
	if p.Location == nil || p.Location.Path != "app.yaml" || p.Location.Line != 7 || p.Location.Column != 3 {
		t.Fatalf("location = %+v", p.Location)
	}
	if p.Count != 4 || p.Unit != "keys" {
		t.Fatalf("count = %d %q", p.Count, p.Unit)
	}
}

func TestC02_011_ProblemActionsReachRunNextSteps(t *testing.T) {
	out, _, _ := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error {
			task.Problem("stale", evo.Next(evo.Command("zq", "prune", "--apply")))
			return nil
		})
	})
	if len(out.Conclusion().Actions) == 0 {
		t.Fatal("an Action on a Problem did not reach the run's next steps")
	}
}

func TestC02_012_StableProblemCodesAreDistinctAndNonEmpty(t *testing.T) {
	a := fmt.Sprint(evo.ProblemCodeVerificationUnsatisfied)
	b := fmt.Sprint(evo.ProblemCodeDuplicateSiblingName)
	if a == "" || b == "" || a == b {
		t.Fatalf("codes %q %q", a, b)
	}
}

func TestC02_013_ResolutionsOfADoneTask(t *testing.T) {
	distinct := map[evo.Resolution]bool{
		evo.ResolutionExecuted: true, evo.ResolutionAlreadySatisfied: true, evo.ResolutionNoWork: true,
	}
	if len(distinct) != 3 {
		t.Fatal("Resolution values collide")
	}
	_, _, executed := concluded(t, returning(nil))
	_, _, satisfied := concluded(t, func(task *evo.TaskHandle) {
		task.Verify(func(context.Context) (bool, error) { return true, nil })
		task.Define(func(context.Context) error { return nil })
	})
	if executed.Resolution != evo.ResolutionExecuted || satisfied.Resolution != evo.ResolutionAlreadySatisfied {
		t.Fatalf("resolutions %v %v", executed.Resolution, satisfied.Resolution)
	}
}

func TestC02_014_EntityStateSetIsTenDistinctValues(t *testing.T) {
	states := []evo.EntityState{
		evo.Pending, evo.Running, evo.Done, evo.Failed, evo.Blocked, evo.Cancelled,
		evo.NotStarted, evo.Skipped, evo.Incomplete, evo.Empty,
	}
	seen := map[evo.EntityState]bool{}
	for _, s := range states {
		if s == "" || seen[s] {
			t.Fatalf("state %q empty or duplicated", s)
		}
		seen[s] = true
	}
}

func TestC02_015_ExitCodesAreFixed(t *testing.T) {
	if evo.ExitOK != 0 || evo.ExitBlocked != 1 || evo.ExitFailed != 2 || evo.ExitCancelled != 130 {
		t.Fatalf("exit codes %d %d %d %d", evo.ExitOK, evo.ExitBlocked, evo.ExitFailed, evo.ExitCancelled)
	}
}
