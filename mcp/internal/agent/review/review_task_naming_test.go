package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// API-046: evo.Exclude("...") whose reason names an already-satisfied
// condition instead of a policy choice.

func excludeSrc(reason string) string {
	return `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	r.Task("install").Define(func(ctx context.Context) error {
		return evo.Exclude("` + reason + `")
	})
}
`
}

var excludeAlreadyUpToDateSrc = excludeSrc("already up to date")

func TestAPI046_ExcludeAlreadyUpToDate_Fires(t *testing.T) {
	res := review.GoSource("adopt.go", excludeAlreadyUpToDateSrc)
	f := findingByID(t, res, "API-046")
	if f.Severity != "warning" {
		t.Fatalf("API-046 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "Verify") {
		t.Fatalf("API-046 suggestion does not name Verify: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "settles Satisfied") {
		t.Fatalf("API-046 suggestion does not name the Satisfied outcome: %q", f.Suggestion)
	}
}

func TestAPI046_ExcludeBareWordReasons_Fire(t *testing.T) {
	cases := []string{"already latest", "unchanged", "current", "latest", "up to date"}
	for _, reason := range cases {
		t.Run(reason, func(t *testing.T) {
			findingByID(t, review.GoSource("adopt.go", excludeSrc(reason)), "API-046")
		})
	}
}

func TestAPI046_ReasonThatIsNotAlreadySatisfied_StaysSilent(t *testing.T) {
	for _, reason := range []string{"no Go module", "protected"} {
		t.Run(reason, func(t *testing.T) {
			for _, f := range review.GoSource("adopt.go", excludeSrc(reason)).Findings {
				if f.RuleID == "API-046" {
					t.Fatalf("false positive API-046 on %q: %+v", reason, f)
				}
			}
		})
	}
}

func TestAPI046_PreOneZeroPin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("adopt.go", excludeAlreadyUpToDateSrc, "0.6.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-046" {
			t.Fatalf("false positive API-046 for a pre-1.0.0 pin: %+v", f)
		}
	}
}

func TestAPI046_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	findingByID(t, review.GoSource("adopt.go", excludeAlreadyUpToDateSrc), "API-046")

	const remediated = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, installed, latest string) {
	r.Task("install").Verify(func(ctx context.Context) (bool, error) {
		return installed == latest, nil
	}).Define(func(ctx context.Context) error { return nil })
}
`
	for _, f := range review.GoSource("adopt.go", remediated).Findings {
		if f.RuleID == "API-046" {
			t.Fatalf("API-046 still fires after remediation: %+v", f)
		}
	}
}

// API-045: a Task name that is a bare subject/category label (ZYS-838's own
// "file integrity") is not one independently meaningful action — the
// finding must name the semantic distinction, not merely grammar, and offer
// the exact corrected verb+object spelling.

const subjectOnlyTaskNameSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("file integrity").Define(checkIntegrity)
}
`

func TestAPI045_SubjectOnlyTaskName_Fires(t *testing.T) {
	res := review.GoSource("check.go", subjectOnlyTaskNameSrc)
	f := findingByID(t, res, "API-045")
	if f.Severity != "warning" {
		t.Fatalf("API-045 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Message, "names a subject, not the work") {
		t.Fatalf("API-045 message does not name the semantic distinction: %q", f.Message)
	}
	if !strings.Contains(f.Suggestion, `"check file integrity"`) {
		t.Fatalf("API-045 suggestion does not offer the corrected verb+object name: %q", f.Suggestion)
	}
}

const containerTaskNameSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("fix").Define(fixAll)
}
`

func TestAPI045_ContainerTaskName_Fires(t *testing.T) {
	res := review.GoSource("fix.go", containerTaskNameSrc)
	f := findingByID(t, res, "API-045")
	if !strings.Contains(f.Message, "organize") {
		t.Fatalf("API-045 message does not describe the container shape: %q", f.Message)
	}
	if !strings.Contains(f.Suggestion, "Group") && !strings.Contains(f.Suggestion, "Sequence") {
		t.Fatalf("API-045 suggestion does not name Group/Sequence: %q", f.Suggestion)
	}
}

const verbObjectTaskNameSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run, g *evo.Group) {
  r.Task("check file integrity").Define(checkIntegrity)
  g.Task("format Python").Define(checkIntegrity)
  g.Task("stabilize Go source").Define(checkIntegrity)
  g.Task("lint Go").Define(checkIntegrity)
  g.Task("check Python").Define(checkIntegrity)
  g.Task("build application icons").Define(checkIntegrity)
}
`

func TestAPI045_VerbObjectTaskNames_StaySilent(t *testing.T) {
	res := review.GoSource("good.go", verbObjectTaskNameSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-045" {
			t.Fatalf("false positive API-045 on a verb+object Task name: %+v", f)
		}
	}
}

func TestAPI045_ObservationsUnderOneTask_StaySilent(t *testing.T) {
	// The "check file integrity" fixture (ZYS-838 acceptance): several
	// internal observations answer one user-meaningful question and stay
	// Facts/problems under one Task, never sibling Tasks.
	const src = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(task *evo.Task) {
  task.Fact("merge markers", "none found")
  task.Fact("symlinks", "valid")
  task.Problem("generated file looks stale", evo.Code("stale"))
}
`
	res := review.GoSource("integrity.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-045" {
			t.Fatalf("false positive API-045 on Facts/Problem under one Task: %+v", f)
		}
	}
}

// API-050: a Task whose literal name is a generic phase/category word
// (fix/check/classify/resolve/finalize — ZYS-937) sequences two or more
// independently erroring steps in its own Define callback instead of
// performing one action itself — it exists primarily to own child-looking
// work or force a row (zq's own fix/check command family,
// internal/app/app.go:80's a.task("fix", ...)).

const phaseTaskOwningChildWorkSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("fix").Define(func(ctx context.Context) error {
    if err := fixGoImports(); err != nil {
      return err
    }
    if err := fixGoFormatting(); err != nil {
      return err
    }
    return nil
  })
}
func fixGoImports() error { return nil }
func fixGoFormatting() error { return nil }
`

func TestAPI049_PhaseTaskOwningChildWork_Fires(t *testing.T) {
	res := review.GoSource("fix.go", phaseTaskOwningChildWorkSrc)
	f := findingByID(t, res, "API-050")
	if f.Severity != "warning" {
		t.Fatalf("API-050 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Message, "own child-looking work") {
		t.Fatalf("API-050 message does not name the semantic distinction: %q", f.Message)
	}
	if !strings.Contains(f.Suggestion, "Group(") {
		t.Fatalf("API-050 suggestion does not name the corrected Group(...) shape: %q", f.Suggestion)
	}
}

// The same shape, declared through a var instead of a chained call, must
// fire identically — API-050 tracks the Task("word")/.Define(...) pairing
// through a local variable, not only a single chained expression.
const phaseTaskOwningChildWorkViaVarSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  t := r.Task("check")
  t.Define(func(ctx context.Context) error {
    if err := checkMergeMarkers(); err != nil {
      return err
    }
    if err := checkSymlinks(); err != nil {
      return err
    }
    return nil
  })
}
func checkMergeMarkers() error { return nil }
func checkSymlinks() error { return nil }
`

func TestAPI049_PhaseTaskOwningChildWorkViaVar_Fires(t *testing.T) {
	res := review.GoSource("check.go", phaseTaskOwningChildWorkViaVarSrc)
	findingByID(t, res, "API-050")
}

const phaseTaskSingleStepSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("check").Define(func(ctx context.Context) error {
    if err := verify(); err != nil {
      return err
    }
    return nil
  })
}
func verify() error { return nil }
`

func TestAPI049_PhaseTaskSingleStep_StaysSilent(t *testing.T) {
	// One guarded step under a generic phase name is still one action, not
	// a container hiding several — API-050 only fires on 2+ steps.
	res := review.GoSource("check_single.go", phaseTaskSingleStepSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-050" {
			t.Fatalf("false positive API-050 on a single guarded step: %+v", f)
		}
	}
}

const verbObjectTaskMultiStepSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  r.Task("stabilize Go source").Define(func(ctx context.Context) error {
    if err := fixGoImports(); err != nil {
      return err
    }
    if err := fixGoFormatting(); err != nil {
      return err
    }
    return nil
  })
}
func fixGoImports() error { return nil }
func fixGoFormatting() error { return nil }
`

func TestAPI049_VerbObjectTaskMultiStep_StaysSilent(t *testing.T) {
	// Several steps under a real verb+object name are not a phase/category
	// label, so this is not API-050's shape.
	res := review.GoSource("stabilize.go", verbObjectTaskMultiStepSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-050" {
			t.Fatalf("false positive API-050 on a verb+object Task name: %+v", f)
		}
	}
}

const phaseTaskRemediatedSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) {
  fixGroup := r.Group("fix")
  fixGroup.Task("fix Go imports").Define(func(ctx context.Context) error { return fixGoImports() })
  fixGroup.Task("fix Go formatting").Define(func(ctx context.Context) error { return fixGoFormatting() })
}
func fixGoImports() error { return nil }
func fixGoFormatting() error { return nil }
`

func TestAPI049_Remediated_StaysSilent(t *testing.T) {
	// Recheck proof: the Group + verb+object children form from
	// phaseTaskOwningChildWorkSrc's own remediation no longer matches
	// API-050's Task("word").Define(...) shape.
	res := review.GoSource("fix_remediated.go", phaseTaskRemediatedSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-050" {
			t.Fatalf("API-050 still fires after remediation: %+v", f)
		}
	}
}
