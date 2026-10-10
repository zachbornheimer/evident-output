package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

func remedyFindings(t *testing.T, src string) []review.Finding {
	t.Helper()
	return migrationFindings(review.GoSource("remedy.go", evoBody(src)))
}

func TestRemovedRemedy_FoldsIntoFollowingDiagnostic(t *testing.T) {
	found := remedyFindings(t, "func f(task *evo.TaskHandle) {\n\ttask.Next(evo.Label(\"retry\"))\n\ttask.Block(\"refused\")\n}\n")
	if len(found) != 1 {
		t.Fatalf("findings = %+v, want one", found)
	}
	want := `replace task.Next(evo.Label("retry")) task.Block("refused") with task.Block("refused", evo.Next(evo.Label("retry")))`
	if found[0].Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", found[0].Suggestion, want)
	}
	if strings.Contains(found[0].Suggestion, "TODO") {
		t.Fatalf("fold rewrite must carry no placeholder: %q", found[0].Suggestion)
	}
}

func TestRemovedRemedy_OutputFoldsIntoItsOwnDiagnostic(t *testing.T) {
	found := remedyFindings(t, "func f(out *evo.Output) {\n\tout.Fail(\"failed\")\n\tout.NextCommand(\"git\", \"status\")\n}\n")
	if len(found) != 1 || !strings.Contains(found[0].Suggestion, `out.Fail("failed", evo.NextCommand("git", "status"))`) {
		t.Fatalf("findings = %+v, want a fold into out.Fail", found)
	}
}

func TestRemovedRemedy_FallbackKeepsReviewOpenUntilPlaceholderReplaced(t *testing.T) {
	src := evoBody("func f(task *evo.TaskHandle) {\n\ttask.NextCommand(\"git\", \"status\")\n}\n")
	found := migrationFindings(review.GoSource("remedy.go", src))
	if len(found) != 1 || !strings.Contains(found[0].Suggestion, "TODO state why this remedy is suggested") {
		t.Fatalf("findings = %+v, want the placeholder fallback", found)
	}
	applied, ok := tryApplyReplace(src, found[0].Suggestion)
	if !ok {
		t.Fatalf("fallback suggestion is not a replace: %q", found[0].Suggestion)
	}
	again := review.GoSource("remedy.go", applied)
	if !again.RecheckRequired || len(migrationFindings(again)) == 0 {
		t.Fatalf("placeholder shipped without a finding: %+v", again)
	}
	edited := strings.Replace(applied, "TODO state why this remedy is suggested", "working tree not checked", 1)
	if done := review.GoSource("remedy.go", edited); done.RecheckRequired {
		t.Fatalf("edited rewrite is still dirty: %+v", done.Findings)
	}
}

func TestRemovedRemedy_OutputFallbackSaysItInventedTheOwner(t *testing.T) {
	found := remedyFindings(t, "func f(out *evo.Output) {\n\tout.Next(evo.Label(\"retry\"))\n}\n")
	if len(found) != 1 || !strings.Contains(found[0].Message, "no natural owner") {
		t.Fatalf("findings = %+v, want the Output fallback to explain the owner", found)
	}
}

func TestRemovedRemedy_IgnoresReceiversThatAreNotEvoBindings(t *testing.T) {
	for name, src := range map[string]string{
		"unrelated local type named it": "type stepper struct{}\nfunc (stepper) Next(x int) {}\nfunc f(it stepper) {\n\tit.Next(1)\n}\n",
		"task-like name, no evo type":   "type cursor struct{}\nfunc (cursor) NextCommand(a, b string) {}\nfunc f(task cursor) {\n\ttask.NextCommand(\"a\", \"b\")\n}\n",
		"live evo option":               "func f(task *evo.TaskHandle) {\n\ttask.Fail(\"failed\", evo.Next(evo.Label(\"retry\")))\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if found := remedyFindings(t, src); len(found) != 0 {
				t.Fatalf("findings = %+v, want none", found)
			}
		})
	}
}
