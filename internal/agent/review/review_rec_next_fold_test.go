package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const remedyPlaceholder = "TODO state why this remedy is suggested"

func remedySource(imports, body string) string {
	return "package p\n\nimport (\n\t\"context\"\n\t\"errors\"\n\t\"fmt\"\n\t\"testing\"\n\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n\n" +
		"var _ = context.Background\nvar _ = errors.New\nvar _ = fmt.Sprint\nvar _ testing.T\n" + imports + "\n" + body
}

func onlyRemedyFinding(t *testing.T, body string) (review.Finding, string) {
	t.Helper()
	src := remedySource("", body)
	found := migrationFindings(review.GoSource("remedy.go", src))
	if len(found) != 1 {
		t.Fatalf("findings = %+v, want exactly one", found)
	}
	return found[0], src
}

func TestRemovedRemedy_FoldNeverCrossesReceiverOrBlockOrSpread(t *testing.T) {
	for name, body := range map[string]string{
		"testing.T.Fail is not an Output diagnostic":      "func f(t *testing.T) {\n\tout := evo.Init(evo.Config{})\n\tif false {\n\t\tt.Fail()\n\t}\n\tout.NextCommand(\"git\", \"status\")\n}\n",
		"a Task diagnostic does not own an Output remedy": "func f(out *evo.Output, task *evo.TaskHandle) {\n\ttask.Fail(\"failed\")\n\tout.NextCommand(\"git\", \"status\")\n}\n",
		"spread options on the diagnostic":                "func f(task *evo.TaskHandle, opts []evo.ProblemOption) {\n\ttask.Fail(\"failed\", opts...)\n\ttask.NextCommand(\"git\", \"status\")\n}\n",
		"diagnostics only in other branches":              "func f(task *evo.TaskHandle, c bool) {\n\tif c {\n\t\ttask.Block(\"a\")\n\t} else {\n\t\ttask.Fail(\"b\")\n\t}\n\ttask.NextCommand(\"git\", \"status\")\n}\n",
		"diagnostic on a different Task":                  "func f(a, b *evo.TaskHandle) {\n\ta.Fail(\"failed\")\n\tb.NextCommand(\"git\", \"status\")\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := onlyRemedyFinding(t, body)
			if !strings.Contains(f.Suggestion, remedyPlaceholder) {
				t.Fatalf("suggestion = %q, want the placeholder fallback (no fold)", f.Suggestion)
			}
		})
	}
}

func TestRemovedRemedy_SpreadNextArgsAreGuidanceOnly(t *testing.T) {
	f, _ := onlyRemedyFinding(t, "func f(task *evo.TaskHandle, as []evo.Action) {\n\ttask.Block(\"b\")\n\ttask.Next(as...)\n}\n")
	if strings.HasPrefix(f.Suggestion, "replace ") {
		t.Fatalf("suggestion = %q, want guidance (a spread Next has no mechanical rewrite)", f.Suggestion)
	}
}

func TestRemovedRemedy_ChainedNextIsGuidanceOnly(t *testing.T) {
	f, _ := onlyRemedyFinding(t, "func f(task *evo.TaskHandle) {\n\ttask.Next(evo.Label(\"run zq init\")).Fail(\"policy\")\n}\n")
	if strings.HasPrefix(f.Suggestion, "replace ") {
		t.Fatalf("suggestion = %q, want guidance", f.Suggestion)
	}
}

func TestRemovedRemedy_FoldIsOneReplaceKeepingInterveningStatements(t *testing.T) {
	f, src := onlyRemedyFinding(t, "func f(task *evo.TaskHandle) {\n\ttask.Fail(\"failed\")\n\tprintln(\"between\")\n\ttask.Next(evo.Label(\"retry\"))\n}\n")
	want := "replace task.Fail(\"failed\")\n\tprintln(\"between\")\n\ttask.Next(evo.Label(\"retry\")) with task.Fail(\"failed\", evo.Next(evo.Label(\"retry\")))\n\tprintln(\"between\")"
	if f.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", f.Suggestion, want)
	}
	applied, ok := tryApplyReplace(src, f.Suggestion)
	if !ok {
		t.Fatalf("suggestion does not apply: %q", f.Suggestion)
	}
	if res := review.GoSource("remedy.go", applied); len(migrationFindings(res)) != 0 || res.RecheckRequired {
		t.Fatalf("applied fold is still dirty: %+v", res)
	}
}

func TestRemovedRemedy_DefineErrorReturnRecordsAnErrorProblem(t *testing.T) {
	body := "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\treturn errors.New(\"lint failed\")\n\t})\n}\n"
	f, src := onlyRemedyFinding(t, body)
	want := `replace task.NextCommand("zq", "fix") with task.Problem("lint failed", evo.NextCommand("zq", "fix"))`
	if f.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", f.Suggestion, want)
	}
	applied, ok := tryApplyReplace(src, f.Suggestion)
	if !ok {
		t.Fatalf("suggestion does not apply: %q", f.Suggestion)
	}
	if res := review.GoSource("remedy.go", applied); len(migrationFindings(res)) != 0 || res.RecheckRequired {
		t.Fatalf("applied rewrite is still dirty: %+v", res)
	}
}

func TestRemovedRemedy_DefineErrorReturnWithUnknownSummaryKeepsReviewOpen(t *testing.T) {
	body := "func f(task *evo.TaskHandle, err error) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tif err != nil {\n\t\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\t}\n\t\treturn fmt.Errorf(\"%w\", err)\n\t})\n}\n"
	f, src := onlyRemedyFinding(t, body)
	want := `replace task.NextCommand("zq", "fix") with task.Problem("` + remedyPlaceholder + `", evo.NextCommand("zq", "fix"))`
	if f.Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", f.Suggestion, want)
	}
	applied, _ := tryApplyReplace(src, f.Suggestion)
	if res := review.GoSource("remedy.go", applied); !res.RecheckRequired {
		t.Fatalf("placeholder summary shipped with recheck closed: %+v", res)
	}
}

func TestRemovedRemedy_DefineWithoutErrorReturnFallsBackToWarning(t *testing.T) {
	body := "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"git\", \"push\")\n\t\treturn nil\n\t})\n}\n"
	f, _ := onlyRemedyFinding(t, body)
	if !strings.Contains(f.Suggestion, "evo.Severity(evo.SeverityWarning)") {
		t.Fatalf("suggestion = %q, want the warning fallback", f.Suggestion)
	}
}
