package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const instantDoneToolSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func bind(out *evo.Output, path string) {
  out.Task("go@1.25.11").Done(path)
}
`

const instantDoneToolFixedSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func bind(out *evo.Output, path string) {
  t := out.Task("go@1.25.11")
  t.Define(func() error {
    return resolve(path)
  })
}
`

const singletonGroupSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  jobs := out.Group("run")
  jobs.Task("install:fresh-start")
}
`

const singletonGroupFixedSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  t := out.Task("install:fresh-start")
  t.Doing("running install:fresh-start")
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

func TestGoSource_InstantDoneToolRow_EmitsFP005WithSuggestion(t *testing.T) {
	res := review.GoSource("bind.go", instantDoneToolSrc)
	f := findingByID(t, res, "FP-005")
	if f.Suggestion == "" {
		t.Fatal("FP-005 suggestion is empty")
	}
	if f.Line == 0 {
		t.Fatal("FP-005 missing line")
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
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
		{"instant-done", instantDoneToolFixedSrc, "FP-005"},
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output, names []string) {
  jobs := out.Group("run")
  for _, name := range names {
    jobs.Task(name)
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
import evo "github.com/zachbornheimer/evident-output"
type subject struct {
  group *evo.GroupHandle
  classify *evo.TaskHandle
}
func declare(out *evo.Output, name string) subject {
  group := out.Group(name)
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

func TestGoSource_FP005SuggestionNamesDefine(t *testing.T) {
	res := review.GoSource("bind.go", instantDoneToolSrc)
	f := findingByID(t, res, "FP-005")
	if !strings.Contains(f.Suggestion, "Define") {
		t.Fatalf("FP-005 suggestion %q does not name Define", f.Suggestion)
	}
	if strings.Contains(f.Suggestion, "Doing(\"resolving\")") {
		t.Fatalf("FP-005 suggestion %q still prescribes Doing-before-Done theater", f.Suggestion)
	}
}

func TestGoSource_API039SuggestionNamesLoneTask(t *testing.T) {
	res := review.GoSource("run.go", singletonGroupSrc)
	f := findingByID(t, res, "API-039")
	if !strings.Contains(f.Suggestion, "Task") {
		t.Fatalf("API-039 suggestion %q does not name Task", f.Suggestion)
	}
}
