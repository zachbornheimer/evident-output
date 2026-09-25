package evo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDryRun_UnsatisfiedVerifyPlansInsteadOfFailing pins E-097: under a
// dry run or preview, a Task whose Verify is false and whose Define plans
// an Effect failed "postcondition not satisfied" with exit 2, because the
// post-Define re-check observed state the skipped Effect never changed.
// A planned run concludes planned, exit 0.
func TestDryRun_UnsatisfiedVerifyPlansInsteadOfFailing(t *testing.T) {
	for name, cfg := range map[string]evo.Config{
		"dry run": {DryRun: true},
		"preview": {Preview: true},
	} {
		var buf bytes.Buffer
		cfg.Isolated, cfg.StateDir, cfg.Stdout, cfg.Title, cfg.Color, cfg.Plain = true, t.TempDir(), &buf, "dry", evo.ColorNever, true
		out := evo.Init(cfg)
		out.Task("converge").
			Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "container", Quantity: 1}, func(context.Context) error { return nil })
			})
		_ = out.Finish()
		c := out.Conclusion()
		if c.State != evo.StatePlanned || c.ExitCode != evo.ExitOK {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.State, c.ExitCode, evo.StatePlanned, evo.ExitOK, buf.String())
		}
		if strings.Contains(buf.String(), "postcondition not satisfied") {
			t.Errorf("%s: a planned run checked its postcondition:\n%s", name, buf.String())
		}
		_ = out.Close()
	}
}

// TestDryRun_UnsatisfiedVerifyWithNothingPlannedFails pins E-106/E-103:
// a planned run skips the after-Define Verify only for a Task whose Define
// planned a mutation. A Define that plans nothing leaves the real
// post-state, so a false Verify must fail exactly as the real run does —
// a dry run that concludes planned exit 0 here predicts success for a run
// certain to fail.
func TestDryRun_UnsatisfiedVerifyWithNothingPlannedFails(t *testing.T) {
	for name, cfg := range map[string]evo.Config{
		"real":    {},
		"dry run": {DryRun: true},
		"preview": {Preview: true},
	} {
		var buf bytes.Buffer
		cfg.Isolated, cfg.StateDir, cfg.Stdout, cfg.Title, cfg.Color, cfg.Plain = true, t.TempDir(), &buf, "dry", evo.ColorNever, true
		out := evo.Init(cfg)
		out.Task("converge").
			Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(context.Context) error { return nil })
		_ = out.Finish()
		c := out.Conclusion()
		if c.State != evo.StateFailed || c.ExitCode != evo.ExitFailed {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.State, c.ExitCode, evo.StateFailed, evo.ExitFailed, buf.String())
		}
		if !strings.Contains(buf.String(), "postcondition not satisfied") {
			t.Errorf("%s: the unchanged postcondition was not checked:\n%s", name, buf.String())
		}
		_ = out.Close()
	}
}

// TestVerify_SelfResolvedDefineIsNotRechecked pins E-110: a Task whose
// Define resolved it itself (Kept, Skipped, Block) made no change to
// verify, yet the post-Define Verify re-check failed the already-terminal
// Task "postcondition not satisfied", exit 2.
func TestVerify_SelfResolvedDefineIsNotRechecked(t *testing.T) {
	for name, tc := range map[string]struct {
		resolve func(*evo.TaskHandle)
		state   evo.ConclusionState
		exit    int
	}{
		"kept":    {func(t *evo.TaskHandle) { t.Kept(evo.Reason("in use")) }, evo.StateReady, evo.ExitOK},
		"skipped": {func(t *evo.TaskHandle) { t.Skipped(evo.Reason("not needed")) }, evo.StateReady, evo.ExitOK},
		"blocked": {func(t *evo.TaskHandle) { t.Block("refused") }, evo.StateBlocked, evo.ExitBlocked},
	} {
		var buf bytes.Buffer
		out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "self", Color: evo.ColorNever, Plain: true})
		task := out.Task("t")
		task.Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(context.Context) error { tc.resolve(task); return nil })
		_ = out.Finish()
		if c := out.Conclusion(); c.State != tc.state || c.ExitCode != tc.exit || strings.Contains(buf.String(), "postcondition not satisfied") {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d with no re-check\n%s", name, c.State, c.ExitCode, tc.state, tc.exit, buf.String())
		}
		_ = out.Close()
	}
}
