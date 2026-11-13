package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// removedCase is one removed-vocabulary call site and the rewrite review
// must suggest for it.
type removedCase struct {
	rule, body, suggests string
}

// checkRemoved reviews body inside a run func and wants exactly one
// finding of c.rule whose suggestion names c.suggests.
func checkRemoved(t *testing.T, name string, c removedCase) {
	t.Helper()
	src := `package main
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
var _ = context.Background
func run(ctx context.Context) error {
	task := evo.Task("t")
	_ = task
` + c.body + `
	return nil
}
`
	res := review.GoSource("main.go", src)
	var got []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == c.rule {
			got = append(got, f)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0].Suggestion, c.suggests) || !res.RecheckRequired {
		t.Errorf("%s: want one %s suggesting %q, got %+v", name, c.rule, c.suggests, res.Findings)
	}
}

// TestRemovedVocabulary_API064 pins E-122: the Reason options the freeze
// removed are flagged with their rewrite.
func TestRemovedVocabulary_API064(t *testing.T) {
	for name, c := range map[string]removedCase{
		"ForSkip":      {"API-064", `	_ = evo.ForSkip()`, "evo.Reason(name)"},
		"OnTask":       {"API-064", `	_ = evo.OnTask("branches")`, "evo.Reason(name)"},
		"ReasonOption": {"API-064", `	var _ evo.ReasonOption`, "evo.Reason(name)"},
	} {
		checkRemoved(t, name, c)
	}
}
