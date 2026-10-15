package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

const singletonGroupSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  jobs := r.Group("run")
  jobs.Task("install:fresh-start").Define(install)
}
`

const singletonGroupFixedSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("install:fresh-start").Define(install)
}
`

func findingByID(t *testing.T, res review.Result, id string) review.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.RuleID == id {
			return f
		}
	}
	t.Fatalf("missing finding %s in %#v", id, res.Findings)
	return review.Finding{}
}

func TestGoSource_SingletonGroup_EmitsAPI039WithSuggestion(t *testing.T) {
	res := review.GoSource("run.go", singletonGroupSrc)
	f := findingByID(t, res, "API-039")
	if f.Suggestion == "" {
		t.Fatal("API-039 suggestion is empty")
	}
	if f.Line == 0 {
		t.Fatal("API-039 missing line")
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
}

func TestGoSource_AppliedSugarSuggestions_AreClean(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		id   string
	}{
		{"singleton-group", singletonGroupFixedSrc, "API-039"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := review.GoSource("fixed.go", tc.src)
			for _, f := range res.Findings {
				if f.RuleID == tc.id {
					t.Fatalf("applied suggestion still emits %s: %+v", tc.id, f)
				}
			}
			if res.RecheckRequired {
				t.Fatalf("recheck_required=true after fix: %+v", res.Findings)
			}
		})
	}
}

func TestGoSource_GroupLoop_IsNotSingleton(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run, names []string) {
  jobs := r.Group("run")
  for _, name := range names {
    jobs.Task(name).Define(install)
  }
}
`
	res := review.GoSource("loop.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-039" {
			t.Fatalf("loop of Task children must not emit API-039: %+v", f)
		}
	}
}

// A constructor that hands its group back cannot be judged on the children
// it declared: whoever receives the group is where the collection gets
// filled. Counting only this function's own Task calls flagged every
// subject factory as a one-child group.
func TestGoSource_GroupReturnedToCaller_IsNotSingleton(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type subject struct {
  group *evo.Group
  classify *evo.Task
}
func declare(r *evo.Run, name string) subject {
  group := r.Group(name)
  return subject{group: group, classify: group.Task("classify")}
}
`
	res := review.GoSource("factory.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-039" {
			t.Fatalf("a group returned to its caller must not emit API-039: %+v", f)
		}
	}
}

func TestGoSource_API039SuggestionNamesLoneTask(t *testing.T) {
	res := review.GoSource("run.go", singletonGroupSrc)
	f := findingByID(t, res, "API-039")
	if !strings.Contains(f.Suggestion, "Task") {
		t.Fatalf("API-039 suggestion %q does not name Task", f.Suggestion)
	}
}

// A Group given its other children by a Build callback, a nested container
// or another function is not a one-child Group.
func TestGoSource_GroupFilledBeyondItsTaskCalls_IsNotSingleton(t *testing.T) {
	for name, body := range map[string]string{
		"build":   `jobs.Build(func(g *evo.Group) { g.Task("b").Define(install) })`,
		"nested":  `jobs.Sequence("then")`,
		"handoff": `declareMore(jobs)`,
	} {
		t.Run(name, func(t *testing.T) {
			src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  jobs := r.Group("run")
  jobs.Task("a").Define(install)
  ` + body + `
}
`
			for _, f := range review.GoSource("filled.go", src).Findings {
				if f.RuleID == "API-039" {
					t.Fatalf("a Group filled by %s must not emit API-039: %+v", name, f)
				}
			}
		})
	}
}

func TestGoSource_InlineGroupWithOneTask_EmitsAPI039(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Group("run").Task("install:fresh-start").Define(install)
}
`
	findingByID(t, review.GoSource("inline.go", src), "API-039")
}
