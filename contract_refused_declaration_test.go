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
// Explanation states what was refused. Every refusal the contract lists must all take that shape.
func TestEveryRefusedDeclarationWrapsErrInvalidConfig(t *testing.T) {
	refusals := map[string]struct {
		body    evo.RunFunc
		refused string
	}{
		"duplicate sibling": {func(_ context.Context, r *evo.Run) error {
			r.Task("build").Define(func(context.Context) error { return nil })
			r.Task("build").Define(func(context.Context) error { return nil })
			return nil
		}, "duplicate task name"},
		"declare inside Define": {func(_ context.Context, r *evo.Run) error {
			r.Task("outer").Define(func(context.Context) error {
				r.Task("inner").Define(func(context.Context) error { return nil })
				return nil
			})
			return nil
		}, "declared from inside a Task callback"},
		"EffectSpec with Path and Resource": {func(_ context.Context, r *evo.Run) error {
			r.Task("push").Define(func(ctx context.Context) error {
				spec := evo.EffectSpec{Verb: evo.Update, Object: "ref", Quantity: 1, Path: "refs/heads", Resource: "remote"}
				return evo.Effect(ctx, spec, func(context.Context) error { return nil })
			})
			return nil
		}, "sets both Path and Resource"},
		"EffectSpec with Quantity 0": {func(_ context.Context, r *evo.Run) error {
			r.Task("prune").Define(func(ctx context.Context) error {
				spec := evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 0}
				return evo.Effect(ctx, spec, func(context.Context) error { return nil })
			})
			return nil
		}, "Quantity must be > 0"},
		"FileSpec with no Path": {func(_ context.Context, r *evo.Run) error {
			r.Task("write").Define(func(ctx context.Context) error {
				return evo.File(ctx, evo.FileSpec{Contents: []byte("x")})
			})
			return nil
		}, "FileSpec.Path is required"},
		"ExecSpec with no Argv": {func(_ context.Context, r *evo.Run) error {
			r.Task("run").Define(func(ctx context.Context) error {
				_, err := evo.Exec(ctx, evo.ExecSpec{})
				return err
			})
			return nil
		}, "ExecSpec.Argv must name an executable"},
		"Define(nil)": {func(_ context.Context, r *evo.Run) error {
			r.Task("nil").Define(nil)
			return nil
		}, `nil Define callback for task "nil"`},
		"Compute on a Task with a Verify": {func(_ context.Context, r *evo.Run) error {
			made := r.Task("make").Verify(func(context.Context) (bool, error) { return false, nil })
			evo.Compute(made, func(context.Context) (int, error) { return 1, nil })
			return nil
		}, "Compute on a Task with a Verify"},
	}
	for name, tc := range refusals {
		t.Run(name, func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "refused", Output: evo.Plain}, tc.body)
			if !errors.Is(run.Result.Err, evo.ErrInvalidConfig) {
				t.Errorf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", run.Result.Err)
			}
			if !strings.Contains(run.Result.Conclusion.Explanation, tc.refused) {
				t.Errorf("Conclusion.Explanation = %q, want it to state the refusal %q", run.Result.Conclusion.Explanation, tc.refused)
			}
		})
	}
}

// TestDefineNilSettlesItsRowNotStartedWithTheRefusal pins that a Task refused for Define(nil) reads as work the
// refusal took away, not as a forgotten terminal verb: the row is not started and names the refusal.
func TestDefineNilSettlesItsRowNotStartedWithTheRefusal(t *testing.T) {
	run := executeContract(context.Background(), t, evo.Config{Name: "refused", Output: evo.Plain}, func(_ context.Context, r *evo.Run) error {
		r.Task("nil").Define(nil)
		return nil
	})
	text := run.Text()
	if strings.Contains(text, "incomplete") {
		t.Errorf("a Define(nil) row reads as incomplete, want not started:\n%s", text)
	}
	if !strings.Contains(text, `nil Define callback for task "nil"`) {
		t.Errorf("the output does not state the Define(nil) refusal:\n%s", text)
	}
}

// TestResultErrJoinsEveryRefusal pins CONTRACT section 7: every refusal a run records is joined into Result.Err
// in the order recorded, while Conclusion.Explanation names the first. The two refusals the run body makes have
// a fixed order; the one a Task callback makes lands whenever that callback runs, so only its presence is pinned.
func TestResultErrJoinsEveryRefusal(t *testing.T) {
	run := executeContract(context.Background(), t, evo.Config{Name: "refused", Output: evo.Plain}, func(_ context.Context, r *evo.Run) error {
		r.Task("build").Define(func(context.Context) error { return nil })
		r.Task("build").Define(func(context.Context) error { return nil })
		made := r.Task("make").Verify(func(context.Context) (bool, error) { return false, nil })
		evo.Compute(made, func(context.Context) (int, error) { return 1, nil })
		r.Task("prune").Define(func(ctx context.Context) error {
			spec := evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 0}
			return evo.Effect(ctx, spec, func(context.Context) error { return nil })
		})
		return nil
	})
	if !errors.Is(run.Result.Err, evo.ErrInvalidConfig) {
		t.Fatalf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", run.Result.Err)
	}
	got := run.Result.Err.Error()
	duplicate, compute, effect := strings.Index(got, "duplicate sibling name"), strings.Index(got, "Compute on a Task with a Verify"), strings.Index(got, "Quantity must be > 0")
	if duplicate < 0 || compute < 0 || effect < 0 {
		t.Errorf("Result.Err = %q, want it to state the duplicate sibling, the Compute on a Verify and the zero Quantity", got)
	}
	if duplicate > compute {
		t.Errorf("Result.Err = %q states the Compute refusal before the duplicate sibling that was recorded first", got)
	}
	if explanation := run.Result.Conclusion.Explanation; !strings.Contains(explanation, "duplicate task name") {
		t.Errorf("Conclusion.Explanation = %q, want it to name the first refusal, the duplicate task name", explanation)
	}
}
