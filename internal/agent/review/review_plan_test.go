package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

func TestAPI032_PlanAndChanges(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  out.Plan("x")
  out.Changes("x")
}
`
	res := review.GoSource("x.go", src)
	var plan, changes bool
	for _, f := range res.Findings {
		if f.RuleID != "API-032" {
			continue
		}
		if strings.Contains(f.Message, "Plan") || strings.Contains(f.Suggestion, ".Plan") {
			plan = true
			if !suggestsTaskMutation(f.Suggestion) {
				t.Errorf("Plan suggestion must name Task mutation verbs, got %q", f.Suggestion)
			}
		}
		if strings.Contains(f.Message, "Changes") || strings.Contains(f.Suggestion, ".Changes") {
			changes = true
			if !suggestsTaskMutation(f.Suggestion) {
				t.Errorf("Changes suggestion must name Task mutation verbs, got %q", f.Suggestion)
			}
		}
	}
	if !plan || !changes {
		t.Fatalf("want API-032 for Plan and Changes, got %+v", res.Findings)
	}
}

func TestAPI032_LibrarianSnippetReportsItemPlanChanges(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  out.Item("note")
  out.Plan("x")
  out.Changes("x")
}
`
	res := review.GoSource("librarian.go", src)
	var item, plan, changes bool
	for _, f := range res.Findings {
		if f.RuleID != "API-032" {
			continue
		}
		switch {
		case strings.Contains(f.Message, "Item") || strings.Contains(f.Suggestion, ".Item"):
			item = true
		case strings.Contains(f.Message, "Plan") || strings.Contains(f.Suggestion, ".Plan"):
			plan = true
		case strings.Contains(f.Message, "Changes") || strings.Contains(f.Suggestion, ".Changes"):
			changes = true
		}
	}
	if !item || !plan || !changes {
		t.Fatalf("librarian snippet must report Item+Plan+Changes, got %+v", res.Findings)
	}
}

func TestAPI032_PlanSkippedWithoutEvoImport(t *testing.T) {
	src := `package p
func f(out planner) {
  out.Plan("x")
  out.Changes("x")
}
`
	res := review.GoSource("no-evo.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-032" {
			t.Fatalf("Plan/Changes without evident-output import must not be API-032: %+v", res.Findings)
		}
	}
}

func suggestsTaskMutation(s string) bool {
	return strings.Contains(s, "Delete") || strings.Contains(s, "Create") || strings.Contains(s, "Record")
}
