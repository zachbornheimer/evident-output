package evo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestDryRun_UnsatisfiedVerifyPlansInsteadOfFailing pins E-097: under a
// dry run, a Task whose Verify is false and whose Define plans
// an Effect failed "postcondition not satisfied" with exit 2, because the
// post-Define re-check observed state the skipped Effect never changed.
// A planned run concludes planned, exit 0.
func TestDryRun_UnsatisfiedVerifyPlansInsteadOfFailing(t *testing.T) {
	for _, name := range []string{"dry run"} {
		var buf bytes.Buffer
		cfg := Config{Name: "dry", DryRun: true, Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stderr: &buf}}
		out := initRun(cfg, tuning{color: colorNever})
		out.Task("converge").
			Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(ctx context.Context) error {
				return Effect(ctx, EffectSpec{Verb: Update, Object: "container", Quantity: 1}, func(context.Context) error { return nil })
			})
		_ = out.finish()
		c := out.conclusion()
		if c.headline() != headlinePlanned || c.exitCode != exitOK {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.headline(), c.exitCode, headlinePlanned, exitOK, buf.String())
		}
		if strings.Contains(buf.String(), "postcondition not satisfied") {
			t.Errorf("%s: a planned run checked its postcondition:\n%s", name, buf.String())
		}
		_ = out.close()
	}
}

// TestDryRun_UnsatisfiedVerifyWithNothingPlannedFails pins E-106/E-103:
// a planned run skips the after-Define Verify only for a Task whose Define
// planned a mutation. A Define that plans nothing leaves the real
// post-state, so a false Verify must fail exactly as the real run does —
// a dry run that concludes planned exit 0 here predicts success for a run
// certain to fail.
func TestDryRun_UnsatisfiedVerifyWithNothingPlannedFails(t *testing.T) {
	for name, dryRun := range map[string]bool{"real": false, "dry run": true} {
		var buf bytes.Buffer
		cfg := Config{Name: "dry", DryRun: dryRun, Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stderr: &buf}}
		out := initRun(cfg, tuning{color: colorNever})
		out.Task("converge").
			Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(context.Context) error { return nil })
		_ = out.finish()
		c := out.conclusion()
		if c.headline() != headlineFailed || c.exitCode != exitFailed {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.headline(), c.exitCode, headlineFailed, exitFailed, buf.String())
		}
		if !strings.Contains(buf.String(), "postcondition not satisfied") {
			t.Errorf("%s: the unchanged postcondition was not checked:\n%s", name, buf.String())
		}
		_ = out.close()
	}
}
