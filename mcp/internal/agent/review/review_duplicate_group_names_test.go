package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// TestDuplicateGroupNamesUnderOneParentAreFlagged: evo refuses a second
// Group (or Sequence) that reuses a sibling's name, whatever the kinds, so
// API-047 flags two Groups or two Sequences with one name exactly as it flags
// a Group beside a Sequence -- and stays silent across parents and branches.
func TestDuplicateGroupNamesUnderOneParentAreFlagged(t *testing.T) {
	fires := map[string]struct{ body, wantSuggestion string }{
		"two groups": {
			"r.Group(\"build\")\n\tr.Group(\"build\")",
			`r.Group("build group")`,
		},
		"two sequences": {
			"r.Sequence(\"ship\")\n\tr.Sequence(\"ship\")",
			`r.Sequence("ship sequence")`,
		},
		"two groups under a nested parent": {
			"work := r.Group(\"work\")\n\twork.Group(\"lint\")\n\twork.Group(\"lint\")",
			`work.Group("lint group")`,
		},
		"chained declarations": {
			"r.Group(\"build\").Task(\"compile\")\n\tr.Group(\"build\").Task(\"link\")",
			`r.Group("build group")`,
		},
	}
	for name, c := range fires {
		t.Run(name, func(t *testing.T) {
			src := "package p\nimport evo \"github.com/zachbornheimer/evident-output/v2\"\nfunc run(r *evo.Run) {\n\t" + c.body + "\n}\n"
			f := findingByID(t, review.GoSource("run.go", src), "API-047")
			if f.Severity != "error" || !strings.Contains(f.Message, "ErrInvalidConfig") {
				t.Fatalf("API-047 = %+v, want an error naming ErrInvalidConfig", f)
			}
			if !strings.Contains(f.Suggestion, c.wantSuggestion) {
				t.Fatalf("suggestion %q does not name %q", f.Suggestion, c.wantSuggestion)
			}
		})
	}

	silent := map[string]string{
		"different parents":  "a := r.Group(\"a\")\n\tb := r.Group(\"b\")\n\ta.Group(\"build\")\n\tb.Group(\"build\")",
		"different names":    "r.Group(\"build\")\n\tr.Group(\"test\")",
		"task beside group":  "r.Task(\"build\")\n\tr.Group(\"build\")",
		"exclusive branches": "if len(r.Args) > 0 {\n\t\tr.Group(\"build\")\n\t} else {\n\t\tr.Group(\"build\")\n\t}",
	}
	for name, body := range silent {
		t.Run("silent "+name, func(t *testing.T) {
			src := "package p\nimport evo \"github.com/zachbornheimer/evident-output/v2\"\nfunc run(r *evo.Run) {\n\t" + body + "\n}\n"
			for _, f := range review.GoSource("run.go", src).Findings {
				if f.RuleID == "API-047" {
					t.Fatalf("API-047 fired: %+v", f)
				}
			}
		})
	}
}
