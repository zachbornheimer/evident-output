package evo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestMisuseAppearsInResult pins CONTRACT.md §8: misuse is recorded with a
// remedy and appears in Result. Result.Err wraps the exported sentinel so
// errors.Is works, Explanation names the misuse, and the remedy for Compute on
// a Task with a Verify talks about Compute and Verify.
func TestMisuseAppearsInResult(t *testing.T) {
	cases := []struct {
		name   string
		body   evo.RunFunc
		remedy []string
	}{
		{
			name: "nil Define callback",
			body: func(_ context.Context, r *evo.Run) error {
				r.Task("build").Define(nil)
				return nil
			},
			remedy: []string{"Define"},
		},
		{
			name: "Compute on a Task with a Verify",
			body: func(_ context.Context, r *evo.Run) error {
				made := r.Task("make").Verify(func(context.Context) (bool, error) { return false, nil })
				evo.Compute(made, func(context.Context) (int, error) { return 1, nil })
				return nil
			},
			remedy: []string{"Compute", "Verify", "make"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "misuse", Output: evo.Plain}, tc.body)
			res := run.Result
			if !errors.Is(res.Err, evo.ErrInvalidConfig) {
				t.Errorf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", res.Err)
			}
			if res.Conclusion.Outcome != evo.Outcome(evo.Failed) {
				t.Errorf("Conclusion.Outcome = %q, want %q", res.Conclusion.Outcome, evo.Failed)
			}
			if !strings.Contains(res.Conclusion.Explanation, "invalid configuration") {
				t.Errorf("Conclusion.Explanation = %q, want it to name the misuse", res.Conclusion.Explanation)
			}
			for _, word := range tc.remedy {
				if !strings.Contains(run.Text(), word) {
					t.Errorf("output does not name %q in its remedy:\n%s", word, run.Text())
				}
			}
		})
	}
}
