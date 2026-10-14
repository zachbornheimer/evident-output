package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

const twoZeroImports = "package p\nimport (\n\t\"context\"\n\tevo \"github.com/zachbornheimer/evident-output/v2\"\n)\n"

func compileFindings(res review.Result) []review.Finding {
	var out []review.Finding
	for _, f := range res.Findings {
		if strings.Contains(f.Message, "does not compile against the 2.0 surface") {
			out = append(out, f)
		}
	}
	return out
}

// A program that imports /v2 and calls something the 2.0 surface lacks is
// never reported clean: the type error is a finding, with the migration
// guide's replacement when the missing name was a 1.x export.
func TestReviewReportsCompileErrorsAgainstTwoZeroSurface(t *testing.T) {
	for name, tc := range map[string]struct{ body, message, suggestion string }{
		"invented function":  {`func f() { evo.Frobnicate("x") }`, "undefined: evo.Frobnicate", "CONTRACT.md"},
		"retired export":     {`func f(ctx context.Context) { evo.Wait() }`, "undefined: evo.Wait", "instead:"},
		"wrong arity":        {`func f(r *evo.Run) { r.Task("a", "b") }`, "too many arguments", "CONTRACT.md"},
		"wrong callback":     {`func f(r *evo.Run) { r.Task("a").Define(func() error { return nil }) }`, "cannot use", "CONTRACT.md"},
		"unknown field":      {`var cfg = evo.Config{Verbosity: 2}`, "unknown field Verbosity", "CONTRACT.md"},
		"package-level Task": {`func f() { evo.Task("a").Define(func(context.Context) error { return nil }) }`, "does not compile", ""},
	} {
		t.Run(name, func(t *testing.T) {
			res := review.GoSource("p.go", twoZeroImports+tc.body+"\n")
			if !res.RecheckRequired {
				t.Fatalf("a program that does not compile is reported clean: %+v", res.Findings)
			}
			if name == "package-level Task" {
				return // reported by the entry-point rewrite, not a bare compile error
			}
			found := compileFindings(res)
			if len(found) == 0 || !strings.Contains(found[0].Message, tc.message) || !strings.Contains(found[0].Suggestion, tc.suggestion) {
				t.Fatalf("want a compile finding mentioning %q and %q, got %+v", tc.message, tc.suggestion, res.Findings)
			}
		})
	}
}

// The check judges the 2.0 API, not the fragment: a file that leaves its
// helpers to other files, or uses another module's package, stays clean.
func TestReviewSurfaceCheckIgnoresFragmentNoise(t *testing.T) {
	src := twoZeroImports + `
func f(ctx context.Context, r *evo.Run) error {
	cmd := cobra.Command{Use: "x"}
	_ = cmd
	r.Task("check").Define(func(ctx context.Context) error { return helperFromAnotherFile(ctx) })
	return nil
}
`
	src = strings.Replace(src, "import (", "import (\n\t\"github.com/spf13/cobra\"", 1)
	if res := review.GoSource("p.go", src); len(res.Findings) != 0 {
		t.Fatalf("fragment noise reported: %s", describeFindings(res.Findings))
	}
}

// Bare package-level constructors are flagged and rewritten to the run's r.
func TestReviewRewritesPackageLevelHandlesToRun(t *testing.T) {
	src := twoZeroImports + `
func run(ctx context.Context, r *evo.Run) error {
	evo.Task("a").Define(func(context.Context) error { return nil })
	g := evo.Group("g")
	g.Task("b").Define(func(context.Context) error { return nil })
	evo.Sequence("s")
	return nil
}
`
	var suggestions []string
	for _, f := range review.GoSource("p.go", src).Findings {
		suggestions = append(suggestions, f.Suggestion)
	}
	for _, want := range []string{
		`replace evo.Task("a") with r.Task("a")`,
		`replace evo.Group("g") with r.Group("g")`,
		`replace evo.Sequence("s") with r.Sequence("s")`,
	} {
		if !strings.Contains(strings.Join(suggestions, "\n"), want) {
			t.Errorf("no suggestion %q in:\n%s", want, strings.Join(suggestions, "\n"))
		}
	}
}
