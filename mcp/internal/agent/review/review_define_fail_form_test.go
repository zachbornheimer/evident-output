package review_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// ZYS-1182 owner decision A: inside a Define callback, Fail adds structured
// diagnostics and the callback still returns the error Wait() reports.

func defineCallback(inner string) string {
	return "func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tif err := run(); err != nil {\n" +
		inner + "\t\t}\n\t\treturn nil\n\t})\n}\n"
}

func TestAPI040_FailCarryingInformationThenReturnErrIsClean(t *testing.T) {
	for name, fail := range map[string]string{
		"detail and remedy": `task.Fail("lint failed", evo.Detail(err.Error()), evo.NextCommand("zq", "fix"))`,
		"remedy only":       `task.Fail("lint failed", evo.Next(evo.Label("retry")))`,
		"detail only":       `task.Fail("lint failed", evo.Detail(err.Error()))`,
	} {
		t.Run(name, func(t *testing.T) {
			res := review.GoSource("a.go", remedySource("", defineCallback("\t\t\t"+fail+"\n\t\t\treturn err\n")))
			if len(res.Findings) != 0 || res.RecheckRequired {
				t.Fatalf("recheck=%v findings=%+v, want clean", res.RecheckRequired, res.Findings)
			}
		})
	}
}

func TestAPI040_FailThatAddsNothingStillFires(t *testing.T) {
	res := review.GoSource("a.go", remedySource("", defineCallback("\t\t\ttask.Fail(\"lint failed\")\n\t\t\treturn err\n")))
	if !hasRule(res, "API-040") {
		t.Fatalf("want API-040, got %+v", res.Findings)
	}
}

func TestAPI034_FailWithInformationThenReturnNilStillFires(t *testing.T) {
	res := review.GoSource("a.go", remedySource("", defineCallback(
		"\t\t\ttask.Fail(\"lint failed\", evo.Detail(err.Error()), evo.NextCommand(\"zq\", \"fix\"))\n\t\t\treturn nil\n")))
	if !hasRule(res, "API-034") {
		t.Fatalf("want API-034, got %+v", res.Findings)
	}
}

func TestRemovedRemedy_DefineRewriteKeepsReturningTheError(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"literal return": {
			"func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\treturn errors.New(\"lint failed\")\n\t})\n}\n",
			`replace task.NextCommand("zq", "fix") return errors.New("lint failed") with task.Fail("lint failed", evo.NextCommand("zq", "fix")); return errors.New("lint failed")`,
		},
		"guarded by if err != nil": {
			"func f(task *evo.TaskHandle) {\n\ttask.Define(func(ctx context.Context) error {\n\t\tif err := run(); err != nil {\n\t\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\t\treturn err\n\t\t}\n\t\treturn nil\n\t})\n}\n",
			`replace task.NextCommand("zq", "fix") return err with task.Fail(err.Error(), evo.NextCommand("zq", "fix")); return err`,
		},
		"zq shape: wrapped error": {
			"func f(task *evo.TaskHandle, name string) {\n\ttask.Define(func(ctx context.Context) error {\n\t\ttask.NextCommand(\"zq\", \"fix\")\n\t\treturn fmt.Errorf(\"lint %s failed\", name)\n\t})\n}\n",
			`replace task.NextCommand("zq", "fix") return fmt.Errorf("lint %s failed", name) with failure := fmt.Errorf("lint %s failed", name); task.Fail(failure.Error(), evo.NextCommand("zq", "fix")); return failure`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f, src := onlyRemedyFinding(t, c.body)
			if f.Suggestion != c.want {
				t.Fatalf("suggestion = %q\n want %q", f.Suggestion, c.want)
			}
			if !strings.Contains(f.Message, "still returns the error") {
				t.Fatalf("message must say the callback still returns the error (Wait() result): %q", f.Message)
			}
			applied, ok := tryApplyReplace(src, f.Suggestion)
			if !ok {
				t.Fatalf("suggestion does not apply: %q", f.Suggestion)
			}
			if res := review.GoSource("remedy.go", applied); len(res.Findings) != 0 || res.RecheckRequired {
				t.Fatalf("applied rewrite does not converge: recheck=%v findings=%+v\n%s", res.RecheckRequired, res.Findings, applied)
			}
		})
	}
}

// A wrapper that embeds an evo handle has the handle's methods: its Next call
// is the removed one, never silently clean.
func TestRemovedRemedy_EmbeddingWrapperIsNotProvenNonEvo(t *testing.T) {
	for name, decl := range map[string]string{
		"pointer embed":    "type wrapper struct{ *evo.TaskHandle }\n",
		"value embed":      "type wrapper struct{ evo.TaskHandle }\n",
		"output embed":     "type wrapper struct{ *evo.Output }\n",
		"transitive embed": "type inner struct{ *evo.TaskHandle }\ntype wrapper struct{ inner }\n",
	} {
		t.Run(name, func(t *testing.T) {
			res := review.GoSource("w.go", evoBody(decl+"func f(w wrapper) {\n\tw.NextCommand(\"git\", \"status\")\n}\n"))
			if len(migrationFindings(res)) == 0 || !res.RecheckRequired {
				t.Fatalf("recheck=%v findings=%+v, want an open finding", res.RecheckRequired, res.Findings)
			}
		})
	}
}

// A type name the file does not declare may be declared (and embed an evo
// handle) in another file: it proves nothing.
func TestRemovedRemedy_TypeDeclaredElsewhereIsUnproven(t *testing.T) {
	files := map[string]string{
		"a.go": remedyPkgHeader + "type wrapper struct{ *evo.TaskHandle }\n",
		"b.go": remedyPkgHeader + "var _ = evo.Init\nfunc f(w wrapper) {\n\tw.NextCommand(\"git\", \"status\")\n}\n",
	}
	res := review.GoPackageAt(files, "")
	if len(migrationFindings(res)) == 0 || !res.RecheckRequired {
		t.Fatalf("recheck=%v findings=%+v, want an open finding", res.RecheckRequired, res.Findings)
	}
}

func TestRemovedRemedy_OneLineRendererLeavesStringLiteralsAlone(t *testing.T) {
	body := "func f(task *evo.TaskHandle) {\n\ttask.Block(\n\t\t\"keep ( this , ) and ,) as is\",\n\t\tevo.Severity(evo.SeverityError),\n\t)\n\ttask.NextCommand(\"zq\", \"fix\")\n}\n"
	f, src := onlyRemedyFinding(t, body)
	applied, ok := tryApplyReplace(src, f.Suggestion)
	if !ok {
		t.Fatalf("suggestion does not apply: %q", f.Suggestion)
	}
	if !strings.Contains(applied, `"keep ( this , ) and ,) as is"`) {
		t.Fatalf("string literal was altered:\n%s", applied)
	}
}

func TestRemovedRemedy_MultilineRawStringDeclinesTheRewrite(t *testing.T) {
	body := "func f(task *evo.TaskHandle) {\n\ttask.Block(`two\nlines`)\n\ttask.NextCommand(\"zq\", \"fix\")\n}\n"
	src := remedySource("", body)
	res := review.GoSource("remedy.go", src)
	found := migrationFindings(res)
	if len(found) != 1 || strings.HasPrefix(found[0].Suggestion, "replace ") || !res.RecheckRequired {
		t.Fatalf("recheck=%v findings=%+v, want one open guidance-only finding", res.RecheckRequired, found)
	}
}

func TestRemovedRemedy_CommentInsideMultilineDiagnosticIsNeverFlattened(t *testing.T) {
	body := "func f(task *evo.TaskHandle) {\n\ttask.Block(\n\t\t\"boom\", // why it blocks\n\t\tevo.Severity(evo.SeverityError),\n\t)\n\ttask.NextCommand(\"zq\", \"fix\")\n}\n"
	src := remedySource("", body)
	res := review.GoSource("remedy.go", src)
	found := migrationFindings(res)
	if len(found) != 1 || !res.RecheckRequired {
		t.Fatalf("recheck=%v findings=%+v, want one open finding", res.RecheckRequired, found)
	}
	if applied, ok := tryApplyReplace(src, found[0].Suggestion); ok {
		if _, err := parser.ParseFile(token.NewFileSet(), "x.go", applied, 0); err != nil || !strings.Contains(applied, "// why it blocks") {
			t.Fatalf("applied rewrite lost or broke the comment (%v):\n%s", err, applied)
		}
	}
}

func TestRemovedRemedy_NoArgNextIsReportedOnlyForProvenEvoReceivers(t *testing.T) {
	res := review.GoSource("n.go", evoBody("func f(task *evo.TaskHandle) {\n\ttask.Next()\n}\n"))
	if found := migrationFindings(res); len(found) != 1 || !res.RecheckRequired {
		t.Fatalf("recheck=%v findings=%+v, want task.Next() reported", res.RecheckRequired, res.Findings)
	}
	iter := "type rows struct{}\nfunc (rows) Next() bool { return false }\nfunc g(r rows) {\n\tr.Next()\n}\nfunc h(c cursorLike) {\n\tc.Next()\n}\n"
	if found := remedyFindings(t, iter); len(found) != 0 {
		t.Fatalf("iterator Next() was reported: %+v", found)
	}
}

func TestRemovedRemedy_FindingLineIsWhereTheEditStarts(t *testing.T) {
	src := remedySource("", "func f(task *evo.TaskHandle) {\n\ttask.Block(\n\t\t\"boom\",\n\t\tevo.Severity(evo.SeverityError),\n\t)\n\ttask.NextCommand(\"zq\", \"fix\")\n}\n")
	found := migrationFindings(review.GoSource("remedy.go", src))
	if len(found) != 1 {
		t.Fatalf("findings = %+v", found)
	}
	want := 1 + strings.Count(src[:strings.Index(src, "task.Block(")], "\n")
	if found[0].Line != want {
		t.Fatalf("Line = %d, want %d (the first line of the replaced text)", found[0].Line, want)
	}
}

// The test applier must not mistake " with " inside a literal for the
// separator, nor let a literal's spaces match a whitespace run.
func TestTryApplyReplace_HandlesWithInsideLiterals(t *testing.T) {
	src := "x := f(\"retry with backoff\")\n"
	got, ok := tryApplyReplace(src, `replace f("retry with backoff") with g("wait with care")`)
	if !ok || got != "x := g(\"wait with care\")\n" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := tryApplyReplace("x := f(\"retry  with backoff\")\n", `replace f("retry with backoff") with g()`); ok {
		t.Fatal("a literal's single space matched a whitespace run")
	}
}

// Block then return err keeps the row blocked and Wait() returns err (errors.Is
// holds); Block then return nil is the form that loses the error's identity.
func TestAPI040_BlockCarryingInformationThenReturnErrIsClean(t *testing.T) {
	for name, block := range map[string]string{
		"detail and remedy": `task.Block("dirty working tree", evo.Detail(err.Error()), evo.NextCommand("git", "status"))`,
		"remedy only":       `task.Block("dirty working tree", evo.NextCommand("git", "status"))`,
	} {
		t.Run(name, func(t *testing.T) {
			res := review.GoSource("a.go", remedySource("", defineCallback("\t\t\t"+block+"\n\t\t\treturn err\n")))
			if len(res.Findings) != 0 || res.RecheckRequired {
				t.Fatalf("recheck=%v findings=%+v, want clean", res.RecheckRequired, res.Findings)
			}
		})
	}
}

func TestAPI040_BareBlockThenReturnErrSaysItAddsNothing(t *testing.T) {
	res := review.GoSource("a.go", remedySource("", defineCallback("\t\t\ttask.Block(\"dirty\")\n\t\t\treturn err\n")))
	var f review.Finding
	for _, x := range res.Findings {
		if x.RuleID == "API-040" {
			f = x
		}
	}
	if f.RuleID == "" {
		t.Fatalf("want API-040, got %+v", res.Findings)
	}
	if strings.Contains(f.Message, "dropped") || !strings.Contains(f.Message, "Block") || !strings.Contains(f.Message, "only restates the returned error") {
		t.Fatalf("message = %q", f.Message)
	}
	if strings.Contains(f.Suggestion, "return nil") {
		t.Fatalf("suggestion teaches the lossy form: %q", f.Suggestion)
	}
}

func TestDOM018_BlockSuggestionReturnsTheError(t *testing.T) {
	src := evoBody("func f(task *evo.TaskHandle, err error) error {\n\ttask.Block(err.Error(), evo.Cause(err))\n\treturn err\n}\n")
	for _, f := range review.GoSource("d.go", src).Findings {
		if f.RuleID == "DOM-018" {
			if strings.Contains(f.Suggestion, "return nil") || !strings.Contains(f.Suggestion, "return err") {
				t.Fatalf("suggestion = %q", f.Suggestion)
			}
			return
		}
	}
	t.Fatal("no DOM-018 finding")
}
