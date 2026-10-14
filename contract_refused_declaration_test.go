package evo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestEveryRefusedDeclarationWrapsErrInvalidConfig pins CONTRACT section 7: a declaration or spec Evo refuses
// has one shape however it was refused. Result.Err wraps ErrInvalidConfig, so errors.Is finds it, and the
// Explanation names it. Five refusals the contract lists must all take that shape.
func TestEveryRefusedDeclarationWrapsErrInvalidConfig(t *testing.T) {
	refusals := map[string]evo.RunFunc{
		"duplicate sibling": func(_ context.Context, r *evo.Run) error {
			r.Task("build").Define(func(context.Context) error { return nil })
			r.Task("build").Define(func(context.Context) error { return nil })
			return nil
		},
		"declare inside Define": func(_ context.Context, r *evo.Run) error {
			r.Task("outer").Define(func(context.Context) error {
				r.Task("inner").Define(func(context.Context) error { return nil })
				return nil
			})
			return nil
		},
		"EffectSpec with Path and Resource": func(_ context.Context, r *evo.Run) error {
			r.Task("push").Define(func(ctx context.Context) error {
				spec := evo.EffectSpec{Verb: evo.Update, Object: "ref", Quantity: 1, Path: "refs/heads", Resource: "remote"}
				return evo.Effect(ctx, spec, func(context.Context) error { return nil })
			})
			return nil
		},
		"EffectSpec with Quantity 0": func(_ context.Context, r *evo.Run) error {
			r.Task("prune").Define(func(ctx context.Context) error {
				spec := evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 0}
				return evo.Effect(ctx, spec, func(context.Context) error { return nil })
			})
			return nil
		},
		"Compute on a Task with a Verify": func(_ context.Context, r *evo.Run) error {
			made := r.Task("make").Verify(func(context.Context) (bool, error) { return false, nil })
			evo.Compute(made, func(context.Context) (int, error) { return 1, nil })
			return nil
		},
	}
	for name, body := range refusals {
		t.Run(name, func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "refused", Output: evo.Plain}, body)
			if !errors.Is(run.Result.Err, evo.ErrInvalidConfig) {
				t.Errorf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", run.Result.Err)
			}
			if !strings.Contains(run.Result.Conclusion.Explanation, "invalid configuration") {
				t.Errorf("Conclusion.Explanation = %q, want it to name the invalid configuration", run.Result.Conclusion.Explanation)
			}
		})
	}
}
