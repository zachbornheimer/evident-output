package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestAllCurrentOperationsSettleSatisfied pins CONTRACT.md §3: a Task whose
// File and Exec calls were all current, and which committed no Effect,
// settles Satisfied; one changed operation or a committed Effect makes it
// Succeeded. Satisfied needs no Verify.
func TestAllCurrentOperationsSettleSatisfied(t *testing.T) {
	w := newFreshnessWorld(t)
	in, copied, generated := w.path("in.txt"), w.path("copy.txt"), w.path("generated.txt")
	w.write(t, "in.txt", "one\n")
	withEffect := false
	operations := func(ctx context.Context) error {
		if err := evo.File(ctx, evo.FileSpec{Path: generated, Contents: []byte("generated\n")}); err != nil {
			return err
		}
		if _, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"cp", in, copied}, Inputs: []string{in}, Outputs: []string{copied}}); err != nil {
			return err
		}
		if !withEffect {
			return nil
		}
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, Object: "cache", Quantity: 1}, func(context.Context) error { return nil })
	}

	steps := []struct {
		name   string
		change func()
		want   evo.Outcome
	}{
		{"first run writes both", func() {}, evo.Succeeded},
		{"unchanged run finds both current", func() {}, evo.Satisfied},
		{"a changed Input re-runs Exec", func() { w.write(t, "in.txt", "two\n") }, evo.Succeeded},
		{"current again", func() {}, evo.Satisfied},
		{"a committed Effect beside current operations", func() { withEffect = true }, evo.Succeeded},
	}
	for _, step := range steps {
		step.change()
		run := w.run(t, operations)
		if got := run.Result.Conclusion.Outcome; got != step.want {
			t.Errorf("%s: Conclusion.Outcome = %q, want %q:\n%s", step.name, got, step.want, run.Text())
		}
	}
}
