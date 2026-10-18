package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// API-047: a Group and a Sequence declared under one parent with the same
// literal name. They share one name space, so the second declaration is
// refused (ErrInvalidConfig). A Task has a name space of its own and may
// share a name with a Group.

const crossKindDuplicateSiblingSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Group("build")
  r.Sequence("build")
}
`

func TestAPI047_GroupAndSequenceSharingName_Fires(t *testing.T) {
	res := review.GoSource("run.go", crossKindDuplicateSiblingSrc)
	f := findingByID(t, res, "API-047")
	if f.Severity != "error" {
		t.Fatalf("API-047 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "r.Sequence(") {
		t.Fatalf("API-047 suggestion does not name the corrected r.Sequence(...) call: %q", f.Suggestion)
	}
	if !strings.Contains(f.Message, "group") || !strings.Contains(f.Message, "ErrInvalidConfig") {
		t.Fatalf("API-047 message does not name the colliding kind and the refusal: %q", f.Message)
	}
}

const crossKindDuplicateSiblingNestedSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  work := r.Group("work")
  work.Group("build")
  work.Sequence("build")
}
`

func TestAPI047_GroupAndSequenceSharingName_FiresUnderNestedParent(t *testing.T) {
	res := review.GoSource("run.go", crossKindDuplicateSiblingNestedSrc)
	f := findingByID(t, res, "API-047")
	if !strings.Contains(f.Suggestion, "work.Sequence(") {
		t.Fatalf("API-047 suggestion does not name the corrected work.Sequence(...) call: %q", f.Suggestion)
	}
}

const taskAndGroupSharingNameSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("build")
  r.Group("build")
}
`

func TestAPI047_TaskAndGroupSharingName_StaysSilent(t *testing.T) {
	// Tasks and containers have separate name spaces: this pair is accepted.
	res := review.GoSource("run.go", taskAndGroupSharingNameSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("API-047 fired for a Task and a Group, which do not collide: %+v", f)
		}
	}
}

const distinctSiblingNamesSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Group("build")
  r.Sequence("test")
}
`

func TestAPI047_DistinctNames_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", distinctSiblingNamesSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("false positive API-047 for distinct sibling names: %+v", f)
		}
	}
}

const crossKindDifferentParentsSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  a := r.Group("a")
  b := r.Group("b")
  a.Group("build")
  b.Sequence("build")
}
`

func TestAPI047_DifferentParents_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", crossKindDifferentParentsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("false positive API-047 across two distinct parents that merely share a literal child name: %+v", f)
		}
	}
}

const crossKindDuplicateSiblingBranchesSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run, cond bool) {
  if cond {
    r.Group("build")
  } else {
    r.Sequence("build")
  }
}
`

func TestAPI047_MutuallyExclusiveBranches_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", crossKindDuplicateSiblingBranchesSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("false positive API-047 across mutually exclusive if/else branches: %+v", f)
		}
	}
}

func TestAPI047_PreTwoZeroPin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run.go", crossKindDuplicateSiblingSrc, "1.1.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("API-047 fired for a pin older than 2.0.0, where Group and Sequence names did not collide: %+v", f)
		}
	}
}

// API-048: a Task re-declared by the same string literal to obtain a later
// dependency reference. The second call is refused as a duplicate sibling
// rather than returning the first handle (CONTRACT.md section 2), so the fix
// is a typed variable (or var (...) block) holding the one handle.

const redeclaredTaskForAfterSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(prune *evo.Group) {
  prune.Task("branches").Define(func(ctx context.Context) error { return nil })
  prune.Task("remote-tracking").After(prune.Task("branches")).Define(func(ctx context.Context) error { return nil })
}
`

func TestAPI048_RedeclaredTaskLiteralForAfter_Fires(t *testing.T) {
	res := review.GoSource("prune.go", redeclaredTaskForAfterSrc)
	f := findingByID(t, res, "API-048")
	if f.Severity != "suggestion" {
		t.Fatalf("API-048 severity = %q, want suggestion", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "var") {
		t.Fatalf("API-048 suggestion does not recommend a typed variable: %q", f.Suggestion)
	}
}

const typedTaskVarSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(prune *evo.Group) {
  var (
    branches = prune.Task("branches")
    remote   = prune.Task("remote-tracking")
  )
  branches.Define(func(ctx context.Context) error { return nil })
  remote.After(branches).Define(func(ctx context.Context) error { return nil })
}
`

func TestAPI048_TypedTaskVar_StaysSilent(t *testing.T) {
	res := review.GoSource("prune_good.go", typedTaskVarSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-048" {
			t.Fatalf("false positive API-048 on a typed Task variable: %+v", f)
		}
	}
}

const oneOffTaskSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(prune *evo.Group) {
  prune.Task("branches").Define(func(ctx context.Context) error { return nil })
}
`

func TestAPI048_TrivialOneOffTask_StaysSilent(t *testing.T) {
	res := review.GoSource("oneoff.go", oneOffTaskSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-048" {
			t.Fatalf("false positive API-048 on a trivial one-off Task: %+v", f)
		}
	}
}

const differentGroupsSameLabelSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(a, b *evo.Group) {
  a.Task("branches").Define(func(ctx context.Context) error { return nil })
  b.Task("branches").Define(func(ctx context.Context) error { return nil })
}
`

func TestAPI048_SameLabelDifferentGroups_StaysSilent(t *testing.T) {
	res := review.GoSource("two_groups.go", differentGroupsSameLabelSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-048" {
			t.Fatalf("false positive API-048 on the same label under two different groups: %+v", f)
		}
	}
}
