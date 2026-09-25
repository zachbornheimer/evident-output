package evo_test

import (
	"bytes"
	"context"
	"errors"
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

// TestVerify_SelfResolvedDefineIsNotRechecked pins E-110 and its
// follow-ups E-112/E-113: a Task whose Define resolved it itself (Kept,
// Skipped, Block) without committing a change has nothing to verify, so
// the post-Define re-check must not fail it. A Define that committed an
// Effect before calling Kept did change state, so its postcondition is
// still checked (E-112). A Define that returns an error replaces its own
// held Kept proposal without a misuse line (E-113).
func TestVerify_SelfResolvedDefineIsNotRechecked(t *testing.T) {
	updateContainer := func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "container", Quantity: 1}, func(context.Context) error { return nil })
	}
	for name, tc := range map[string]struct {
		define  func(context.Context, *evo.TaskHandle) error
		state   evo.ConclusionState
		exit    int
		want    []string
		wantNot []string
	}{
		"kept": {
			define: func(_ context.Context, t *evo.TaskHandle) error { t.Kept(evo.Reason("in use")); return nil },
			state:  evo.StateReady, exit: evo.ExitOK, wantNot: []string{"postcondition not satisfied"},
		},
		"skipped": {
			define: func(_ context.Context, t *evo.TaskHandle) error { t.Skipped(evo.Reason("not needed")); return nil },
			state:  evo.StateReady, exit: evo.ExitOK, wantNot: []string{"postcondition not satisfied"},
		},
		"blocked": {
			define: func(_ context.Context, t *evo.TaskHandle) error { t.Block("refused"); return nil },
			state:  evo.StateBlocked, exit: evo.ExitBlocked, wantNot: []string{"postcondition not satisfied"},
		},
		"effect then kept": {
			define: func(ctx context.Context, t *evo.TaskHandle) error {
				if err := updateContainer(ctx); err != nil {
					return err
				}
				t.Kept(evo.Reason("in use"))
				return nil
			},
			state: evo.StateFailed, exit: evo.ExitFailed, want: []string{"postcondition not satisfied"},
		},
		"effect then skipped": {
			define: func(ctx context.Context, t *evo.TaskHandle) error {
				if err := updateContainer(ctx); err != nil {
					return err
				}
				t.Skipped(evo.Reason("not needed"))
				return nil
			},
			state: evo.StateFailed, exit: evo.ExitFailed, want: []string{"postcondition not satisfied"},
		},
		"kept then error": {
			define: func(_ context.Context, t *evo.TaskHandle) error {
				t.Kept(evo.Reason("in use"))
				return errors.New("boom")
			},
			state: evo.StateFailed, exit: evo.ExitFailed, want: []string{"boom"},
			wantNot: []string{"already resolved", "postcondition not satisfied"},
		},
		"skipped then error": {
			define: func(_ context.Context, t *evo.TaskHandle) error {
				t.Skipped(evo.Reason("n/a"))
				return errors.New("boom")
			},
			state: evo.StateFailed, exit: evo.ExitFailed, want: []string{"boom"},
			wantNot: []string{"already resolved", "postcondition not satisfied"},
		},
	} {
		var buf bytes.Buffer
		out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "self", Color: evo.ColorNever, Plain: true})
		task := out.Task("t")
		task.Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(ctx context.Context) error { return tc.define(ctx, task) })
		_ = out.Finish()
		got := buf.String()
		if c := out.Conclusion(); c.State != tc.state || c.ExitCode != tc.exit {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.State, c.ExitCode, tc.state, tc.exit, got)
		}
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: output lacks %q\n%s", name, w, got)
			}
		}
		for _, w := range tc.wantNot {
			if strings.Contains(got, w) {
				t.Errorf("%s: output has %q\n%s", name, w, got)
			}
		}
		_ = out.Close()
	}
}
