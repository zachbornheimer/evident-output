package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

const legacyHeader = "package p\nimport (\n\t\"context\"\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n"

// TestRemovedTwoZeroRunLevelRewrites pins the rewrite each 1.x run-level
// shape gets, and that the 2.0 shapes that look like them stay silent.
func TestRemovedTwoZeroRunLevelRewrites(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"Exec returned": {
			`func f(ctx context.Context) error { return evo.Exec(ctx, evo.ExecSpec{}) }`,
			`replace return evo.Exec(ctx, evo.ExecSpec{}) with if _, err := evo.Exec(ctx, evo.ExecSpec{}); err != nil { return err }; return nil`,
		},
		"Exec assigned": {
			`func f(ctx context.Context) error { err := evo.Exec(ctx, evo.ExecSpec{}); return err }`,
			`replace err := evo.Exec(ctx, evo.ExecSpec{}) with _, err := evo.Exec(ctx, evo.ExecSpec{})`,
		},
		"Exec reassigned": {
			`func f(ctx context.Context) (err error) { err = evo.Exec(ctx, evo.ExecSpec{}); return }`,
			`replace err = evo.Exec(ctx, evo.ExecSpec{}) with _, err = evo.Exec(ctx, evo.ExecSpec{})`,
		},
		"Exec in a closure": {
			`func f(t *evo.TaskHandle) { t.Define(func(ctx context.Context) error { return evo.Exec(ctx, evo.ExecSpec{}) }) }`,
			`if _, err := evo.Exec(ctx, evo.ExecSpec{}); err != nil { return err }; return nil`,
		},
		"Key statement": {
			`func f(t *evo.TaskHandle) { t.Key("k") }`,
			`delete t.Key("k"); the Task name is its identity`,
		},
		"Bytes operands": {
			`func f(t *evo.TaskHandle, done, total int64) { t.Bytes(done, 4096) }`,
			`replace t.Bytes(done, 4096) with t.Progress(int(done), 4096)`,
		},
		"Basis with App": {
			`func f(ctx context.Context) { _, _ = evo.Exec(ctx, evo.ExecSpec{Basis: []evo.Fingerprint{evo.App()}}) }`,
			`instead: ExecSpec.Inputs for each FSPath and ExecSpec.Values for each Value`,
		},
		"Basis of a FileSpec": {
			`func f(ctx context.Context, n int) error { return evo.File(ctx, evo.FileSpec{Basis: []evo.Fingerprint{evo.Value("n", n)}}) }`,
			`replace Basis: []evo.Fingerprint{evo.Value("n", n)} with Values: map[string]string{"n": fmt.Sprint(n)}`,
		},
		"Verbose printf": {
			`func f(r *evo.Run) { evo.Verbose().Printf("n=%d", 1) }`,
			`replace evo.Verbose().Printf("n=%d", 1) with fmt.Fprintf(r.Text(), "n=%d", 1)`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			res := review.GoSource("legacy.go", legacyHeader+c.body)
			if !strings.Contains(joinSuggestions(res.Findings), c.want) {
				t.Fatalf("want a suggestion %q, got:\n%s", c.want, describeFindings(res.Findings))
			}
		})
	}
}

func TestRemovedTwoZeroRunLevelSilentOnTwoZeroShapes(t *testing.T) {
	const header = "package p\nimport (\n\t\"context\"\n\tevo \"github.com/zachbornheimer/evident-output/v2\"\n)\n"
	for name, body := range map[string]string{
		"Exec returns both":     `func f(ctx context.Context) (evo.ExecResult, error) { return evo.Exec(ctx, evo.ExecSpec{Argv: []string{"x"}}) }`,
		"Exec keeps both":       `func f(ctx context.Context) error { res, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"x"}}); _ = res; return err }`,
		"Exec as a statement":   `func f(ctx context.Context) { evo.Exec(ctx, evo.ExecSpec{Argv: []string{"x"}}) }`,
		"Execute exit":          `func f(ctx context.Context, run evo.RunFunc) int { return evo.Execute(ctx, evo.Config{}, run).ExitCode() }`,
		"Run as a conversion":   `func f(g func(context.Context, *evo.Run) error) evo.RunFunc { return evo.RunFunc(g) }`,
		"Spec with Inputs":      `var _ = evo.ExecSpec{Argv: []string{"x"}, Inputs: []string{"a"}, Values: map[string]string{"k": "v"}}`,
		"Progress on a Task":    `func f(t *evo.Task) { t.Progress(1, 2) }`,
		"Key on a foreign type": `func f(m map[string]int) { _ = m["k"] }`,
	} {
		t.Run(name, func(t *testing.T) {
			res := review.GoSource("two_zero.go", header+body)
			for _, f := range res.Findings {
				if f.RuleID == "API-032" {
					t.Errorf("a 2.0 shape draws a migration finding: %s", f.Message)
				}
			}
		})
	}
}
