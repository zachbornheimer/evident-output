package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

const remedyPlaceholder = "TODO state why this remedy is suggested"

func remedySource(imports, body string) string {
	return "package p\n\nimport (\n\t\"context\"\n\t\"errors\"\n\t\"fmt\"\n\t\"testing\"\n\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n\n" +
		"var _ = context.Background\nvar _ = errors.New\nvar _ = fmt.Sprint\nvar _ testing.T\n\nfunc run() error { return nil }\n" + imports + "\n" + body
}

func onlyRemedyFinding(t *testing.T, body string) (review.Finding, string) {
	t.Helper()
	src := remedySource("", body)
	found := migrationFindings(review.GoSource("remedy.go", src))
	if len(found) != 1 {
		t.Fatalf("findings = %+v, want exactly one", found)
	}
	if strings.Contains(found[0].Suggestion, "\n") {
		t.Fatalf("suggestion is not one line: %q", found[0].Suggestion)
	}
	return found[0], src
}

func TestRemovedRemedy_FoldNeverCrossesReceiverBlockSpreadOrStatements(t *testing.T) {
	for name, body := range map[string]string{
		"testing.T.Fail is not an Output diagnostic":      "func f(t *testing.T) {\n\tout := evo.Init(evo.Config{})\n\tif false {\n\t\tt.Fail()\n\t}\n\tout.NextCommand(\"git\", \"status\")\n}\n",
		"a Task diagnostic does not own an Output remedy": "func f(out *evo.Output, task *evo.TaskHandle) {\n\ttask.Fail(\"failed\")\n\tout.NextCommand(\"git\", \"status\")\n}\n",
		"spread options on the diagnostic":                "func f(task *evo.TaskHandle, opts []evo.ProblemOption) {\n\ttask.Fail(\"failed\", opts...)\n\ttask.NextCommand(\"git\", \"status\")\n}\n",
		"diagnostics only in other branches":              "func f(task *evo.TaskHandle, c bool) {\n\tif c {\n\t\ttask.Block(\"a\")\n\t} else {\n\t\ttask.Fail(\"b\")\n\t}\n\ttask.NextCommand(\"git\", \"status\")\n}\n",
		"diagnostic on a different Task":                  "func f(a, b *evo.TaskHandle) {\n\ta.Fail(\"failed\")\n\tb.NextCommand(\"git\", \"status\")\n}\n",
		"a statement between":                             "func f(task *evo.TaskHandle) {\n\ttask.Fail(\"failed\")\n\tprintln(\"between\")\n\ttask.Next(evo.Label(\"retry\"))\n}\n",
		"a branch between (following diagnostic)":         "func f(task *evo.TaskHandle, c bool) {\n\ttask.NextCommand(\"zq\", \"fix\")\n\tif c {\n\t\treturn\n\t}\n\ttask.Block(\"stop\")\n}\n",
		"a comment between":                               "func f(task *evo.TaskHandle) {\n\ttask.Fail(\"boom\") // why\n\ttask.NextCommand(\"zq\", \"fix\")\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := onlyRemedyFinding(t, body)
			if !strings.Contains(f.Suggestion, remedyPlaceholder) {
				t.Fatalf("suggestion = %q, want the placeholder fallback (no fold)", f.Suggestion)
			}
		})
	}
}

func TestRemovedRemedy_SpreadNextArgsAreGuidanceThatKeepsTheSpread(t *testing.T) {
	for name, body := range map[string]string{
		"Next":        "func f(task *evo.TaskHandle, as []evo.Action) {\n\ttask.Block(\"b\")\n\ttask.Next(as...)\n}\n",
		"NextCommand": "func f(task *evo.TaskHandle, args []string) {\n\ttask.Block(\"b\")\n\ttask.NextCommand(\"git\", args...)\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := onlyRemedyFinding(t, body)
			if strings.HasPrefix(f.Suggestion, "replace ") || !strings.Contains(f.Suggestion, "...)") {
				t.Fatalf("suggestion = %q, want guidance that keeps the `...`", f.Suggestion)
			}
		})
	}
}

func TestRemovedRemedy_ChainedNextIsGuidanceOnly(t *testing.T) {
	f, _ := onlyRemedyFinding(t, "func f(task *evo.TaskHandle) {\n\ttask.Next(evo.Label(\"run zq init\")).Fail(\"policy\")\n}\n")
	if strings.HasPrefix(f.Suggestion, "replace ") {
		t.Fatalf("suggestion = %q, want guidance", f.Suggestion)
	}
}

func TestRemovedRemedy_MultilineDiagnosticFoldsAsOneLineEdit(t *testing.T) {
	f, src := onlyRemedyFinding(t, "func f(task *evo.TaskHandle) {\n\ttask.Block(\n\t\t\"boom\",\n\t\tevo.Severity(evo.SeverityError),\n\t)\n\ttask.NextCommand(\"zq\", \"fix\")\n}\n")
	applied, ok := tryApplyReplace(src, f.Suggestion)
	if !ok {
		t.Fatalf("suggestion does not apply: %q", f.Suggestion)
	}
	if res := review.GoSource("remedy.go", applied); len(migrationFindings(res)) != 0 || res.RecheckRequired {
		t.Fatalf("applied fold is still dirty: %+v\n%s", res, applied)
	}
}

// A return that is not provably on this Next call's path must not become a Fail:
// the remedy would fire on a path it does not explain.
func TestRemovedRemedy_DefineErrorReturnNeverCrossesControlFlow(t *testing.T) {
	for name, body := range map[string]string{
		"unrelated later branch":                        "func f(task *evo.TaskHandle, x, y bool) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tif x {\n\t\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\t}\n\t\tif y {\n\t\t\treturn errors.New(\"disk full\")\n\t\t}\n\t\treturn nil\n\t})\n}\n",
		"err may be nil":                                "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\terr := run()\n\t\ttask.NextCommand(\"zq\", \"retry\")\n\t\treturn err\n\t})\n}\n",
		"return after a guard":                          "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"zq\", \"retry\")\n\t\tif err := run(); err != nil {\n\t\t\treturn fmt.Errorf(\"run failed\")\n\t\t}\n\t\treturn nil\n\t})\n}\n",
		"errors.Join may be nil":                        "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"zq\", \"retry\")\n\t\treturn errors.Join(errors.New(\"a\"))\n\t})\n}\n",
		"success-path relief (zq clean.go:204)":         "func f(task *evo.TaskHandle, relief func() (evo.Action, bool)) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tif action, ok := relief(); ok {\n\t\t\ttask.Next(action)\n\t\t}\n\t\ttask.Summary(\"checked\")\n\t\treturn run()\n\t})\n}\n",
		"conditional hint, error later (zq app.go:483)": "func f(task *evo.TaskHandle, on bool) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tif on {\n\t\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\t}\n\t\treturn fmt.Errorf(\"%w\", run())\n\t})\n}\n",
		"nested closure return":                         "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tgo func() error {\n\t\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\t\treturn errors.New(\"x\")\n\t\t}()\n\t\treturn nil\n\t})\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := onlyRemedyFinding(t, body)
			if !strings.Contains(f.Suggestion, "evo.Severity(evo.SeverityWarning)") || !strings.Contains(f.Suggestion, remedyPlaceholder) {
				t.Fatalf("suggestion = %q, want the warning-severity placeholder fallback", f.Suggestion)
			}
			if strings.Contains(f.Suggestion, ".Fail(") {
				t.Fatalf("suggestion = %q, must not become a Fail", f.Suggestion)
			}
		})
	}
}

func TestRemovedRemedy_DefineWithoutErrorReturnFallsBackToWarning(t *testing.T) {
	body := "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"git\", \"push\")\n\t\treturn nil\n\t})\n}\n"
	f, _ := onlyRemedyFinding(t, body)
	if !strings.Contains(f.Suggestion, "evo.Severity(evo.SeverityWarning)") {
		t.Fatalf("suggestion = %q, want the warning fallback", f.Suggestion)
	}
}
