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
