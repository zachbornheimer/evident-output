package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

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

func TestDOM018_SuggestionKeepsTheErrorInDetailOnly(t *testing.T) {
	src := evoBody("func f(err error) error {\n\treturn evo.Fail(err.Error(), evo.Detail(err.Error()))\n}\n")
	for _, f := range review.GoSource("d.go", src).Findings {
		if f.RuleID == "DOM-018" {
			if strings.Contains(f.Suggestion, "return nil") || !strings.Contains(f.Suggestion, `evo.Fail("<what failed>", evo.Detail(err.Error()))`) {
				t.Fatalf("suggestion = %q", f.Suggestion)
			}
			return
		}
	}
	t.Fatal("no DOM-018 finding")
}
