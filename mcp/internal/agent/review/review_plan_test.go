package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
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
			if !suggestsCurrentReplacement(f.Suggestion) {
				t.Errorf("Plan suggestion must steer to 1.1 replacements (Effect/File/Fact), got %q", f.Suggestion)
			}
		}
		if strings.Contains(f.Message, "Changes") || strings.Contains(f.Suggestion, ".Changes") {
			changes = true
			if !suggestsCurrentReplacement(f.Suggestion) {
				t.Errorf("Changes suggestion must steer to 1.1 replacements (Effect/File/Fact), got %q", f.Suggestion)
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

// suggestsCurrentReplacement reports whether a suggestion names the 1.1
// replacements for retired mutation reporting and never a removed verb
// (Task.Record* was removed in 1.1, ZYS-974).
func suggestsCurrentReplacement(s string) bool {
	return strings.Contains(s, "evo.Effect") && strings.Contains(s, "evo.File") &&
		strings.Contains(s, "Task.Fact") && !strings.Contains(s, "Record")
}

func TestAPI032_OKAndBecauseSteerAwayFromRemovedDone(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  t := out.Task("x")
  t.OK().Because("why")
}
`
	res := review.GoSource("ok.go", src)
	var ok, because bool
	for _, f := range res.Findings {
		if f.RuleID != "API-032" {
			continue
		}
		switch {
		case strings.Contains(f.Message, "OK was retired"):
			ok = true
		case strings.Contains(f.Message, "Because was retired"):
			because = true
		default:
			continue
		}
		if strings.Contains(f.Suggestion, "Done(") {
			t.Errorf("suggestion steers to removed TaskHandle.Done: %q", f.Suggestion)
		}
		if !strings.Contains(f.Suggestion, "Define(") {
			t.Errorf("suggestion must name Define: %q", f.Suggestion)
		}
	}
	if !ok || !because {
		t.Fatalf("want API-032 for OK and Because, got %+v", res.Findings)
	}
}
