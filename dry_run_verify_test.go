package evo

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestDryRun_UnsatisfiedVerifyPlansInsteadOfFailing pins E-097: under a
// dry run or preview, a Task whose Verify is false and whose Define plans
// an Effect failed "postcondition not satisfied" with exit 2, because the
// post-Define re-check observed state the skipped Effect never changed.
// A planned run concludes planned, exit 0.
func TestDryRun_UnsatisfiedVerifyPlansInsteadOfFailing(t *testing.T) {
	for name, cfg := range map[string]Config{
		"dry run": {DryRun: true},
		"preview": {preview: true},
	} {
		var buf bytes.Buffer
		cfg.isolated, cfg.stateDir, cfg.stdout, cfg.title, cfg.color, cfg.plain = true, t.TempDir(), &buf, "dry", colorNever, true
		out := initRun(cfg)
		out.Task("converge").
			Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(ctx context.Context) error {
				return Effect(ctx, EffectSpec{Verb: Update, Object: "container", Quantity: 1}, func(context.Context) error { return nil })
			})
		_ = out.finish()
		c := out.conclusion()
		if c.state != statePlanned || c.exitCode != exitOK {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.state, c.exitCode, statePlanned, exitOK, buf.String())
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
	for name, cfg := range map[string]Config{
		"real":    {},
		"dry run": {DryRun: true},
		"preview": {preview: true},
	} {
		var buf bytes.Buffer
		cfg.isolated, cfg.stateDir, cfg.stdout, cfg.title, cfg.color, cfg.plain = true, t.TempDir(), &buf, "dry", colorNever, true
		out := initRun(cfg)
		out.Task("converge").
			Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(context.Context) error { return nil })
		_ = out.finish()
		c := out.conclusion()
		if c.state != stateFailed || c.exitCode != exitFailed {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.state, c.exitCode, stateFailed, exitFailed, buf.String())
		}
		if !strings.Contains(buf.String(), "postcondition not satisfied") {
			t.Errorf("%s: the unchanged postcondition was not checked:\n%s", name, buf.String())
		}
		_ = out.close()
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
		return Effect(ctx, EffectSpec{Verb: Update, Object: "container", Quantity: 1}, func(context.Context) error { return nil })
	}
	for name, tc := range map[string]struct {
		define  func(context.Context, *Task) error
		state   conclusionState
		exit    int
		want    []string
		wantNot []string
	}{
		"kept": {
			define: func(_ context.Context, t *Task) error { t.skipped(reason("in use")); return nil },
			state:  stateReady, exit: exitOK, wantNot: []string{"postcondition not satisfied"},
		},
		"skipped": {
			define: func(_ context.Context, t *Task) error { t.skipped(reason("not needed")); return nil },
			state:  stateReady, exit: exitOK, wantNot: []string{"postcondition not satisfied"},
		},
		"blocked": {
			define: func(_ context.Context, t *Task) error { t.block("refused"); return nil },
			state:  stateBlocked, exit: exitBlocked, wantNot: []string{"postcondition not satisfied"},
		},
		"effect then kept": {
			define: func(ctx context.Context, t *Task) error {
				if err := updateContainer(ctx); err != nil {
					return err
				}
				t.skipped(reason("in use"))
				return nil
			},
			state: stateFailed, exit: exitFailed, want: []string{"postcondition not satisfied"},
		},
		"effect then skipped": {
			define: func(ctx context.Context, t *Task) error {
				if err := updateContainer(ctx); err != nil {
					return err
				}
				t.skipped(reason("not needed"))
				return nil
			},
			state: stateFailed, exit: exitFailed, want: []string{"postcondition not satisfied"},
		},
		"kept then error": {
			define: func(_ context.Context, t *Task) error {
				t.skipped(reason("in use"))
				return errors.New("boom")
			},
			state: stateFailed, exit: exitFailed, want: []string{"boom"},
			wantNot: []string{"already resolved", "postcondition not satisfied"},
		},
		"skipped then error": {
			define: func(_ context.Context, t *Task) error {
				t.skipped(reason("n/a"))
				return errors.New("boom")
			},
			state: stateFailed, exit: exitFailed, want: []string{"boom"},
			wantNot: []string{"already resolved", "postcondition not satisfied"},
		},
	} {
		var buf bytes.Buffer
		out := initRun(Config{isolated: true, stateDir: t.TempDir(), stdout: &buf, title: "self", color: colorNever, plain: true})
		task := out.Task("t")
		task.Verify(func(context.Context) (bool, error) { return false, nil }).
			Define(func(ctx context.Context) error { return tc.define(ctx, task) })
		_ = out.finish()
		got := buf.String()
		if c := out.conclusion(); c.state != tc.state || c.exitCode != tc.exit {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.state, c.exitCode, tc.state, tc.exit, got)
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
		_ = out.close()
	}
}
