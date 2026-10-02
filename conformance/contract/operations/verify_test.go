package operations_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func constant(v bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) { return v, nil }
}

func commitEffect(ctx context.Context) error {
	return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "branch", Quantity: 1},
		func(context.Context) error { return nil })
}

// verified runs one Verify-backed task and returns its snapshot and the run.
func verified(t *testing.T, cfg func(*evo.Config), build func(task *evo.TaskHandle)) (*evo.Output, evo.TaskSnapshot) {
	t.Helper()
	var mutate []func(*evo.Config)
	if cfg != nil {
		mutate = append(mutate, cfg)
	}
	out, _ := harness.New(t, mutate...)
	task := out.Task("job")
	build(task)
	_ = task.Wait()
	_ = out.Finish()
	return out, harness.MustFind(t, out, "job")
}

func TestC05_002_BeforeChecksAreANDedAndAllTrueSkipsTheCallback(t *testing.T) {
	ran := false
	_, allTrue := verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(constant(true)).Verify(constant(true))
		task.Define(func(context.Context) error { ran = true; return nil })
	})
	if ran || allTrue.Resolution != evo.ResolutionAlreadySatisfied {
		t.Fatalf("all-true ran=%v resolution=%v", ran, allTrue.Resolution)
	}
	ran = false
	verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(constant(true)).Verify(constant(false))
		task.Define(func(context.Context) error { ran = true; return nil })
	})
	if !ran {
		t.Fatal("one false check must run the callback")
	}
}

func TestC05_003_FalseAfterCheckFailsWithUnsatisfiedCode(t *testing.T) {
	out, task := verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(constant(false))
		task.Define(func(context.Context) error { return nil })
	})
	if task.State != evo.Failed || out.Conclusion().ExitCode != evo.ExitFailed {
		t.Fatalf("state %v exit %d", task.State, out.Conclusion().ExitCode)
	}
	if len(task.Problems) != 1 ||
		task.Problems[0].Code != fmt.Sprint(evo.ProblemCodeVerificationUnsatisfied) ||
		!strings.Contains(task.Problems[0].Summary, "postcondition not satisfied") {
		t.Fatalf("problems = %+v", task.Problems)
	}
}

func TestC05_004_CallbackThatEstablishesStateSatisfiesTheAfterCheck(t *testing.T) {
	established := false
	_, task := verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(func(context.Context) (bool, error) { return established, nil })
		task.Define(func(context.Context) error { established = true; return nil })
	})
	if task.State != evo.Done || task.Resolution != evo.ResolutionExecuted {
		t.Fatalf("state %v resolution %v", task.State, task.Resolution)
	}
}

func TestC05_005_DryRunSkipsAfterCheckOnlyForPlannedWork(t *testing.T) {
	dry := func(c *evo.Config) { c.DryRun = true }
	_, planned := verified(t, dry, func(task *evo.TaskHandle) {
		task.Verify(constant(false))
		task.Define(commitEffect)
	})
	if planned.State != evo.Done {
		t.Fatalf("planned task = %v, want Done (mutation never ran)", planned.State)
	}
	_, idle := verified(t, dry, func(task *evo.TaskHandle) {
		task.Verify(constant(false))
		task.Define(func(context.Context) error { return nil })
	})
	if idle.State != evo.Failed {
		t.Fatalf("task that planned nothing = %v, want Failed", idle.State)
	}
}

func TestC05_006_SelfResolvingCallbackSkipsTheAfterCheck(t *testing.T) {
	_, skipped := verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(constant(false))
		task.Define(func(context.Context) error { task.Skipped(evo.Reason("protected")); return nil })
	})
	_, blocked := verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(constant(false))
		task.Define(func(context.Context) error { task.Block("refused"); return nil })
	})
	if skipped.State != evo.Skipped || blocked.State != evo.Blocked {
		t.Fatalf("skipped=%v blocked=%v", skipped.State, blocked.State)
	}
}

func TestC05_007_CommittedEffectBeforeSelfResolutionStillRunsAfterCheck(t *testing.T) {
	_, task := verified(t, nil, func(task *evo.TaskHandle) {
		task.Verify(constant(false))
		task.Define(func(ctx context.Context) error {
			if err := commitEffect(ctx); err != nil {
				return err
			}
			task.Skipped(evo.Reason("rest protected"))
			return nil
		})
	})
	if task.State != evo.Failed {
		t.Fatalf("state = %v, want Failed (state changed, check ran)", task.State)
	}
}

func TestC05_008_SkippedThenErrorFailsWithThatErrorAndNoMisuseLine(t *testing.T) {
	boom := errors.New("boom")
	out, task := verified(t, nil, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Skipped(evo.Reason("protected")); return boom })
	})
	if task.State != evo.Failed {
		t.Fatalf("state = %v", task.State)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish reported misuse: %v", err)
	}
}
