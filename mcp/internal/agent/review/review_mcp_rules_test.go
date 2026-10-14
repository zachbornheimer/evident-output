package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// API-040: an ending built by evo.Fail/Refuse/Exclude and then discarded.

const discardedEndingSrc = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, doWork func() error) {
	r.Task("resolve").Define(func(ctx context.Context) error {
		if err := doWork(); err != nil {
			evo.Fail("resolve failed")
			return err
		}
		return nil
	})
}
`

func TestAPI040_DiscardedEnding_Fires(t *testing.T) {
	res := review.GoSource("app.go", discardedEndingSrc)
	f := findingByID(t, res, "API-040")
	if f.Severity != "error" {
		t.Fatalf("API-040 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "return evo.Fail(") {
		t.Fatalf("API-040 suggestion does not say to return the ending: %q", f.Suggestion)
	}
}

const returnedEndingSrc = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, doWork func() error) {
	r.Task("resolve").Define(func(ctx context.Context) error {
		if err := doWork(); err != nil {
			return evo.Fail("resolve failed", evo.Detail(err.Error()))
		}
		return nil
	})
}
`

func TestAPI040_ReturnedEnding_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", returnedEndingSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-040" {
			t.Fatalf("false positive API-040 on a returned ending: %+v", f)
		}
	}
}

// API-041: a goroutine resolving a predeclared Task with no Define inside.

const goroutinePredeclaredSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context, r *evo.Run) error {
  t := r.Task("a")
  go func() {
    t.Doing("working")
    t.Progress(1, 2)
  }()
  return nil
}
`

func TestAPI041_GoroutineResolvesPredeclaredTask_Fires(t *testing.T) {
	res := review.GoSource("run.go", goroutinePredeclaredSrc)
	f := findingByID(t, res, "API-041")
	if f.Severity != "error" {
		t.Fatalf("API-041 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "Group") || !strings.Contains(f.Suggestion, "Define") {
		t.Fatalf("API-041 suggestion does not name Group.Task/Define: %q", f.Suggestion)
	}
}

const goroutineWithDefineSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context, r *evo.Run) error {
  t := r.Task("a")
  go func() {
    t.Define(func(ctx context.Context) error { return nil })
  }()
  return nil
}
`

const goroutineProgressOnDefinedTaskSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context, r *evo.Run) error {
  t := r.Task("a").Define(func(ctx context.Context) error { return nil })
  go func() {
    t.Progress(1, 2)
  }()
  return nil
}
`

func TestAPI041_ProgressOnDefinedTask_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", goroutineProgressOnDefinedTaskSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-041" {
			t.Fatalf("false positive API-041 when the Task was given Define: %+v", f)
		}
	}
}

func TestAPI041_GoroutineWithDefine_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", goroutineWithDefineSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-041" {
			t.Fatalf("false positive API-041 when the closure calls Define: %+v", f)
		}
	}
}

// API-042: an evo.Effect with a nil or no-op callback.

const nilEffectCallbackSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "module", Quantity: 2}, nil)
}
`

func TestAPI042_NilEffectCallback_Fires(t *testing.T) {
	res := review.GoSource("setup.go", nilEffectCallbackSrc)
	f := findingByID(t, res, "API-042")
	if f.Severity != "error" {
		t.Fatalf("API-042 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "task.Fact") {
		t.Fatalf("API-042 suggestion does not name task.Fact for information-only work: %q", f.Suggestion)
	}
}

const noOpLiteralEffectCallbackSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "worktree", Quantity: 1}, func(context.Context) error { return nil })
}
`

func TestAPI042_NoOpLiteralEffectCallback_Fires(t *testing.T) {
	res := review.GoSource("clean.go", noOpLiteralEffectCallbackSrc)
	findingByID(t, res, "API-042")
}

const noOpNamedEffectCallbackSrc = `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, name string, n int) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "module", Quantity: n}, func(context.Context) error {
    return installedCount(name, n)
  })
}
func installedCount(name string, count int) error {
  if name == "" || count < 1 {
    return fmt.Errorf("expected installed modules")
  }
  return nil
}
`

func TestAPI042_NoOpDelegatedEffectCallback_Fires(t *testing.T) {
	res := review.GoSource("setup.go", noOpNamedEffectCallbackSrc)
	findingByID(t, res, "API-042")
}

const realEffectCallbackSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "module", Quantity: 2}, func(ctx context.Context) error {
    return installPackages(ctx)
  })
}
func installPackages(ctx context.Context) error { return invokeUV(ctx) }
`

func TestAPI042_RealWorkEffectCallback_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", realEffectCallbackSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-042" {
			t.Fatalf("false positive API-042 on an Effect callback that calls real work: %+v", f)
		}
	}
}

// API-043: a plural EffectSpec.Object literal.

const pluralEffectObjectSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "worktrees", Quantity: 1}, removeWorktree)
}
`

func TestAPI043_PluralEffectObjectLiteral_Fires(t *testing.T) {
	res := review.GoSource("clean.go", pluralEffectObjectSrc)
	f := findingByID(t, res, "API-043")
	if f.Severity != "warning" {
		t.Fatalf("API-043 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, `"worktree"`) {
		t.Fatalf("API-043 suggestion does not name the singular: %q", f.Suggestion)
	}
	if strings.Contains(f.Message, "Affected") || strings.Contains(f.Suggestion, "Affected") {
		t.Fatalf("API-043 must not teach the removed evo.Affected: %+v", f)
	}
}

const singularEffectObjectSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "worktree", Quantity: 1}, removeWorktree)
}
`

func TestAPI043_SingularEffectObjectLiteral_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", singularEffectObjectSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-043" {
			t.Fatalf("false positive API-043 on singular object: %+v", f)
		}
	}
}

const pluralNonEvoObjectFieldSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type Row struct{ Object string }
func run(_ *evo.Task) Row { return Row{Object: "worktrees"} }
`

func TestAPI043_PluralNonEffectSpecObjectField_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", pluralNonEvoObjectFieldSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-043" {
			t.Fatalf("false positive API-043 on a non-EffectSpec Object field: %+v", f)
		}
	}
}

// API-044: declaring work with Define and then blocking on a channel the
// callback feeds turns the declaring stack into a waiter.

const channelWaitAroundDefineSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func defineAndWait(task *evo.Task, fn func(context.Context) error) error {
  done := make(chan error, 1)
  task.Define(func(ctx context.Context) error {
    err := fn(ctx)
    done <- err
    return err
  })
  return <-done
}
`

func TestAPI044_ChannelWaitWrapper_Fires(t *testing.T) {
	res := review.GoSource("adapter.go", channelWaitAroundDefineSrc)
	f := findingByID(t, res, "API-044")
	if f.Severity != "error" {
		t.Fatalf("API-044 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "After") || !strings.Contains(f.Suggestion, "Computed.Get") {
		t.Fatalf("API-044 suggestion does not name After and Computed.Get: %q", f.Suggestion)
	}
}

const plainDefineNoChannelSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(task *evo.Task, fn func(context.Context) error) {
  task.Define(fn)
}
`

// A channel the function never receives from is not a wait.
const sendOnlyChannelSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(task *evo.Task, events chan<- string, fn func(context.Context) error) {
  progress := make(chan string, 1)
  task.Define(func(ctx context.Context) error {
    progress <- "started"
    return fn(ctx)
  })
  events <- "declared"
}
`

func TestAPI044_NoWait_StaysSilent(t *testing.T) {
	for name, src := range map[string]string{"plain define": plainDefineNoChannelSrc, "send only": sendOnlyChannelSrc} {
		res := review.GoSource("good.go", src)
		for _, f := range res.Findings {
			if f.RuleID == "API-044" {
				t.Fatalf("false positive API-044 on %s: %+v", name, f)
			}
		}
	}
}

// TAX-003: an Exclude reason that restates its verb.

func TestTAX003_ReasonRestatesVerb_Fires(t *testing.T) {
	res := review.GoSource("run.go", excludeSrc("skipped"))
	f := findingByID(t, res, "TAX-003")
	if f.Severity != "warning" {
		t.Fatalf("TAX-003 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Message, "restates Exclude") {
		t.Fatalf("TAX-003 message does not call out the restated verb: %q", f.Message)
	}
	if !strings.Contains(f.Suggestion, "evo.Exclude(") {
		t.Fatalf("TAX-003 suggestion does not name evo.Exclude: %q", f.Suggestion)
	}
}

func TestTAX003_ReasonNamingWhy_StaysSilent(t *testing.T) {
	assertNoRule(t, review.GoSource("run.go", excludeSrc("protected branch")), "TAX-003")
}

func TestTAX003_TestFile_StaysSilent(t *testing.T) {
	assertNoRule(t, review.GoSource("run_test.go", excludeSrc("skipped")), "TAX-003")
}

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

// API-049: a Define callback discards its scheduler-provided context and
// passes a captured outer ctx into cancellable work instead (ZYS-938).

const defineDiscardsSchedulerCtxSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, task *evo.Task) {
  task.Define(func(context.Context) error {
    return doWork(ctx)
  })
}
func doWork(ctx context.Context) error { return nil }
`

func TestAPI049_DefineDiscardsSchedulerCtx_Fires(t *testing.T) {
	res := review.GoSource("run.go", defineDiscardsSchedulerCtxSrc)
	f := findingByID(t, res, "API-049")
	if f.Severity != "error" {
		t.Fatalf("API-049 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "func(ctx context.Context) error") {
		t.Fatalf("API-049 suggestion does not name the corrected signature: %q", f.Suggestion)
	}
}

const defineUsesOwnCtxSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, task *evo.Task) {
  task.Define(func(ctx context.Context) error {
    return doWork(ctx)
  })
}
func doWork(ctx context.Context) error { return nil }
`

func TestAPI049_DefineUsesOwnCtx_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", defineUsesOwnCtxSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-049" {
			t.Fatalf("false positive API-049 when Define names and uses its own ctx: %+v", f)
		}
	}
}

const defineNoCancellableWorkSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, task *evo.Task) {
  task.Define(func(context.Context) error {
    return doWork()
  })
}
func doWork() error { return nil }
`

func TestAPI049_NoCancellableWork_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", defineNoCancellableWorkSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-049" {
			t.Fatalf("false positive API-049 when the callback never uses the captured outer ctx: %+v", f)
		}
	}
}

func TestAPI049_PreOneZeroPin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run.go", defineDiscardsSchedulerCtxSrc, "0.6.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-049" {
			t.Fatalf("API-049 fired for a pin older than 1.0.0 (ctx-based Define did not exist yet): %+v", f)
		}
	}
}

// A non-evo type that happens to declare its own Define(func(context.Context)
// error) method in the same file as evo usage must not be mistaken for evo's
// Task.Define — a proven false positive.
const defineOnUnrelatedTypeSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)

type Validator struct{}

func (v *Validator) Define(fn func(context.Context) error) {
  _ = fn
}

func run(ctx context.Context, task *evo.Task) {
  v := &Validator{}
  v.Define(func(context.Context) error {
    return doWork(ctx)
  })
}
func doWork(ctx context.Context) error { return nil }
`

func TestAPI049_UnrelatedTypeWithOwnDefineMethod_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", defineOnUnrelatedTypeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-049" {
			t.Fatalf("false positive API-049 on an unrelated type's own Define method: %+v", f)
		}
	}
}

// A callback that discards the scheduler ctx but declares its own local ctx
// (shadowing the captured outer one) before calling cancellable work is a
// correct, common pattern (e.g. intentionally detached background work) —
// ZYS-938's second proven false positive.
const defineShadowsCtxLocallySrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, task *evo.Task) {
  task.Define(func(context.Context) error {
    ctx := context.Background()
    return doWork(ctx)
  })
}
func doWork(ctx context.Context) error { return nil }
`

func TestAPI049_LocalCtxShadow_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", defineShadowsCtxLocallySrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-049" {
			t.Fatalf("false positive API-049 when the callback shadows ctx with its own local before calling cancellable work: %+v", f)
		}
	}
}

// API-051: a loop that flattens structured findings into one joined error
// string, or spawns one fake Task per finding, instead of accumulating them
// with Task.Problem.

const flattenedDiagnosticsLoopSrc = `package p
import (
  "strings"

  evo "github.com/zachbornheimer/evident-output/v2"
)
func blockStagedGolangciFindings(task *evo.Task, findings []finding) error {
  var lines []string
  for _, f := range findings {
    lines = append(lines, formatFinding(f))
  }
  return evo.Fail(strings.Join(lines, "\n"))
}
type finding struct{}
func formatFinding(f finding) string { return "" }
`

func TestAPI051_FlattenedDiagnosticsLoop_Fires(t *testing.T) {
	res := review.GoSource("hook_findings.go", flattenedDiagnosticsLoopSrc)
	f := findingByID(t, res, "API-051")
	if f.Severity != "error" {
		t.Fatalf("API-051 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "task.Problem") {
		t.Fatalf("API-051 suggestion does not name task.Problem: %q", f.Suggestion)
	}
}

const fakeTaskPerFindingSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func reportFileIntegrityIssues(group *evo.Group, findings []finding) {
  for _, f := range findings {
    group.Task(f.File).Define(func(context.Context) error { return evo.Fail(f.Message) })
  }
}
type finding struct {
  File    string
  Message string
}
`

func TestAPI051_FakeTaskPerFinding_Fires(t *testing.T) {
	res := review.GoSource("hook.go", fakeTaskPerFindingSrc)
	f := findingByID(t, res, "API-051")
	if f.Severity != "error" {
		t.Fatalf("API-051 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "task.Problem") {
		t.Fatalf("API-051 suggestion does not name task.Problem: %q", f.Suggestion)
	}
}

const problemAccumulationSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(r *evo.Run, issues []issue) {
  task := r.Task("file integrity")
  for _, issue := range issues {
    task.Problem(issue.Summary,
      evo.On(issue.Path),
      evo.Code(issue.Code),
      evo.Location(issue.Path, issue.Line, 0),
    )
  }
  task.Define(func(context.Context) error { return nil })
}
type issue struct {
  Summary string
  Path    string
  Code    string
  Line    int
}
`

func TestAPI051_ProblemAccumulation_StaysSilent(t *testing.T) {
	res := review.GoSource("hook_good.go", problemAccumulationSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-051" {
			t.Fatalf("false positive API-051 on the recommended task.Problem accumulation: %+v", f)
		}
	}
}

const unrelatedJoinedStringSrc = `package p
import (
  "log"
  "strings"
)
func summarize(names []string) {
  var lines []string
  for _, n := range names {
    lines = append(lines, n)
  }
  log.Println(strings.Join(lines, ", "))
}
`

func TestAPI051_UnrelatedJoinedString_StaysSilent(t *testing.T) {
	res := review.GoSource("summarize.go", unrelatedJoinedStringSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-051" {
			t.Fatalf("false positive API-051 on a joined string never wrapped in errors.New/Fail: %+v", f)
		}
	}
}

func TestAPI051_PreOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("hook_findings.go", flattenedDiagnosticsLoopSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-051" {
			t.Fatalf("API-051 fired for a pin older than 1.1.0 (Task.Problem accumulation did not exist yet): %+v", f)
		}
	}
	res = review.GoSourceAt("hook.go", fakeTaskPerFindingSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-051" {
			t.Fatalf("API-051 fired for a pin older than 1.1.0 (Task.Problem accumulation did not exist yet): %+v", f)
		}
	}
}

// API-053: nested Evo resource acquisition (ZYS-933/ZYS-840).

const nestedResourceDirectSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
  spec := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Quantity: 1, Path: from}
  return evo.Effect(ctx, spec, func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: to, Contents: marker})
  })
}
`

func TestAPI053_NestedFileInsideEffectResourceHold_Fires(t *testing.T) {
	res := review.GoSource("worktree.go", nestedResourceDirectSrc)
	f := findingByID(t, res, "API-053")
	if f.Severity != "error" {
		t.Fatalf("API-053 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "coarser Resource") && !strings.Contains(f.Suggestion, "before starting a second") {
		t.Fatalf("API-053 suggestion does not name the fix: %q", f.Suggestion)
	}
}

const nestedResourceIndirectSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
  spec := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Quantity: 1, Path: from}
  return evo.Effect(ctx, spec, func(ctx context.Context) error {
    return writeMarker(ctx, to, marker)
  })
}
func writeMarker(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI053_NestedFileThroughHelperOneCallAway_Fires(t *testing.T) {
	res := review.GoSource("worktree.go", nestedResourceIndirectSrc)
	findingByID(t, res, "API-053")
}

const nestedResourceEffectInEffectSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func moveTwoWorktrees(ctx context.Context, a, b string) error {
  outer := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Quantity: 1, Path: a}
  return evo.Effect(ctx, outer, func(ctx context.Context) error {
    inner := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Quantity: 1, Resource: b}
    return evo.Effect(ctx, inner, func(ctx context.Context) error { return nil })
  })
}
`

func TestAPI053_NestedEffectInsideEffectResourceHold_Fires(t *testing.T) {
	res := review.GoSource("worktree.go", nestedResourceEffectInEffectSrc)
	findingByID(t, res, "API-053")
}

const sequentialResourceNotNestedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
  if err := evo.File(ctx, evo.FileSpec{Path: to, Contents: marker}); err != nil {
    return err
  }
  spec := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Quantity: 1, Path: from}
  return evo.Effect(ctx, spec, func(ctx context.Context) error {
    return removeDir(from)
  })
}
`

func TestAPI053_SequentialFileThenEffect_StaysSilent(t *testing.T) {
	res := review.GoSource("worktree.go", sequentialResourceNotNestedSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("false positive API-053 on sequential (not nested) File then Effect: %+v", f)
		}
	}
}

const effectWithoutResourceSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func cleanCache(ctx context.Context, path string, marker []byte) error {
  spec := evo.EffectSpec{Object: "cache entry", Verb: evo.Delete, Quantity: 1}
  return evo.Effect(ctx, spec, func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: path, Contents: marker})
  })
}
`

func TestAPI053_EffectWithoutResource_StaysSilent(t *testing.T) {
	res := review.GoSource("cache.go", effectWithoutResourceSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("false positive API-053 when the enclosing Effect never claims a Resource: %+v", f)
		}
	}
}

func TestAPI053_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("worktree.go", nestedResourceDirectSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("API-053 fired for a pin older than 1.1.0 (EffectSpec.Resource did not exist yet): %+v", f)
		}
	}
}

func TestAPI053_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("worktree.go", nestedResourceDirectSrc)
	findingByID(t, res, "API-053")

	after := review.GoSource("worktree.go", sequentialResourceNotNestedSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("API-053 still fires after remediation to sequential File then Effect: %+v", f)
		}
	}
}

// API-054: generic bytes.Buffer/io.MultiWriter/task.Writer plumbing wired
// around a raw os/exec.Cmd to recreate Exec's own capture/liveness, plus
// string-match cancellation detection, instead of using evo.Exec and
// inspecting the returned ExecResult (ZYS-942, ZYS-850's ExecResult;
// zq run_captured_task.go evidence: own bytes.Buffer, task.Writer()
// combined via io.MultiWriter, output-string cancellation match).

const manualCaptureBufferMultiWriterSrc = `package p
import (
  "bytes"
  "io"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func runCapturedTask(task *evo.Task, cmd *exec.Cmd) error {
  var buf bytes.Buffer
  cmd.Stdout = io.MultiWriter(task.Writer(), &buf)
  cmd.Stderr = io.MultiWriter(task.Writer(), &buf)
  if err := cmd.Run(); err != nil {
    return err
  }
  return nil
}
`

func TestAPI054_ManualBufferMultiWriterAroundTaskWriter_Fires(t *testing.T) {
	res := review.GoSource("run_captured_task.go", manualCaptureBufferMultiWriterSrc)
	f := findingByID(t, res, "API-054")
	if f.Severity != "error" {
		t.Fatalf("API-054 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.Exec") || !strings.Contains(f.Suggestion, "ExecResult") {
		t.Fatalf("API-054 suggestion does not name the Exec/ExecResult fix: %q", f.Suggestion)
	}
}

const manualCancellationStringMatchSrc = `package p
import (
  "os/exec"
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func runChecked(task *evo.Task, cmd *exec.Cmd) error {
  cmd.Stdout = task.Writer()
  out, err := cmd.CombinedOutput()
  if err != nil {
    if strings.Contains(string(out), "signal: killed") {
      return context.Canceled
    }
    return err
  }
  return nil
}
`

func TestAPI054_StringMatchCancellationAroundTaskWriter_Fires(t *testing.T) {
	res := review.GoSource("run_checked.go", manualCancellationStringMatchSrc)
	f := findingByID(t, res, "API-054")
	if !strings.Contains(f.Suggestion, "ExecResult") {
		t.Fatalf("API-054 suggestion does not name ExecResult: %q", f.Suggestion)
	}
}

const evoExecResultInspectionSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func runChecked(ctx context.Context, spec evo.ExecSpec) error {
  res, err := evo.Exec(ctx, spec)
  if errors.Is(err, evo.ErrExecNonzeroExit) {
    task.Failf("lint failed: %s", res.Stdout)
    return nil
  }
  return err
}
`

func TestAPI054_EvoExecResultInspection_StaysSilent(t *testing.T) {
	res := review.GoSource("run_checked.go", evoExecResultInspectionSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("false positive API-054 on evo.Exec/ExecResult inspection: %+v", f)
		}
	}
}

const unrelatedBufferMultiWriterNoTaskSrc = `package p
import (
  "bytes"
  "io"
  "os"
)
func teeToFile(f *os.File) io.Writer {
  var buf bytes.Buffer
  return io.MultiWriter(f, &buf)
}
`

func TestAPI054_UnrelatedBufferMultiWriterNoTaskWriter_StaysSilent(t *testing.T) {
	res := review.GoSource("tee.go", unrelatedBufferMultiWriterNoTaskSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("false positive API-054 without any task.Writer()/raw exec.Cmd combination: %+v", f)
		}
	}
}

func TestAPI054_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run_captured_task.go", manualCaptureBufferMultiWriterSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("API-054 fired for a pin older than 1.1.0 (ExecResult did not exist yet): %+v", f)
		}
	}
}

func TestAPI054_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("run_captured_task.go", manualCaptureBufferMultiWriterSrc)
	findingByID(t, res, "API-054")

	after := review.GoSource("run_checked.go", evoExecResultInspectionSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("API-054 still fires after remediation to evo.Exec/ExecResult: %+v", f)
		}
	}
}

const stringLiteralMentionsCmdRunNoRealExecSrc = `package p
import (
  "bytes"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func inspectExecResult(task *evo.Task, res evo.ExecResult) error {
  // note: not the same as hand-rolling cmd.Run( capture ourselves
  var buf bytes.Buffer
  buf.WriteString("cmd.Run( appears only in this comment and string, never as real Go syntax")
  task.Writer().Write(buf.Bytes())
  if res.ExitCode != 0 {
    return evo.ErrExecNonzeroExit
  }
  return nil
}
`

func TestAPI054_StringLiteralAndCommentMentionCmdRun_StaysSilent(t *testing.T) {
	res := review.GoSource("inspect.go", stringLiteralMentionsCmdRunNoRealExecSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("false positive API-054: rawExecCmdSignals matched inside a string literal/comment, not real exec.Cmd syntax: %+v", f)
		}
	}
}

// API-055 (ZYS-931): a caller-managed sync.Mutex/RWMutex Lock/Unlock wrapped
// around an evo.File call — the exact caller lock ZYS-840's automatic
// resource coordination now owns (File claims its own path for writing with
// no caller code).

const manualMutexAroundFileWriteSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  mu   sync.Mutex
  path string
}
func (w *Writer) write(ctx context.Context, contents []byte) error {
  w.mu.Lock()
  defer w.mu.Unlock()
  return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
`

func TestAPI055_ManualMutexAroundFileWrite_Fires(t *testing.T) {
	res := review.GoSource("writer.go", manualMutexAroundFileWriteSrc)
	f := findingByID(t, res, "API-055")
	if f.Severity != "warning" {
		t.Fatalf("API-055 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("API-055 suggestion does not name evo.File as the fix: %q", f.Suggestion)
	}
}

const noManualMutexAroundFileWriteSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  path string
}
func (w *Writer) write(ctx context.Context, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
`

func TestAPI055_NoManualMutexAroundFileWrite_StaysSilent(t *testing.T) {
	res := review.GoSource("writer.go", noManualMutexAroundFileWriteSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("false positive API-055 when File already owns its own coordination: %+v", f)
		}
	}
}

const mutexUnrelatedToEvoFileSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Service struct {
  mu    sync.Mutex
  count int
}
func (s *Service) touch() {
  s.mu.Lock()
  defer s.mu.Unlock()
  s.count++
}
func (s *Service) write(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI055_MutexGuardingUnrelatedDomainState_StaysSilent(t *testing.T) {
	res := review.GoSource("service.go", mutexUnrelatedToEvoFileSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("false positive API-055 on a mutex guarding unrelated domain state: %+v", f)
		}
	}
}

const rwMutexRLockAroundFileReadSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  mu   sync.RWMutex
  path string
}
func (w *Writer) read(ctx context.Context) ([]byte, error) {
  w.mu.RLock()
  defer w.mu.RUnlock()
  var out []byte
  err := evo.File(ctx, evo.FileSpec{Path: w.path, Contents: out})
  return out, err
}
`

func TestAPI055_RWMutexRLockAroundFileRead_Fires(t *testing.T) {
	res := review.GoSource("writer.go", rwMutexRLockAroundFileReadSrc)
	f := findingByID(t, res, "API-055")
	if f.Severity != "warning" {
		t.Fatalf("API-055 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "w.mu.RLock()/w.mu.RUnlock()") {
		t.Fatalf("API-055 suggestion does not name the actual RLock()/RUnlock() pair: %q", f.Suggestion)
	}
	if strings.Contains(f.Suggestion, "w.mu.Lock()/w.mu.Unlock()") {
		t.Fatalf("API-055 suggestion wrongly names Lock()/Unlock() for RWMutex RLock code: %q", f.Suggestion)
	}
}

const rwMutexRLockScopeNarrowsAtRealRUnlockSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  mu   sync.RWMutex
  path string
}
func (w *Writer) read(ctx context.Context) ([]byte, error) {
  w.mu.RLock()
  var out []byte
  err := evo.File(ctx, evo.FileSpec{Path: w.path, Contents: out})
  w.mu.RUnlock()
  if err != nil {
    return nil, err
  }
  return out, nil
}
`

func TestAPI055_RWMutexRLockScopeNarrowsAtRealRUnlock_Fires(t *testing.T) {
	res := review.GoSource("writer.go", rwMutexRLockScopeNarrowsAtRealRUnlockSrc)
	f := findingByID(t, res, "API-055")
	if !strings.Contains(f.Suggestion, "RLock()/w.mu.RUnlock()") {
		t.Fatalf("API-055 suggestion does not name the actual RLock()/RUnlock() pair: %q", f.Suggestion)
	}
}

func TestAPI055_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("writer.go", manualMutexAroundFileWriteSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("API-055 fired for a pin older than 1.1.0 (automatic resource claiming did not exist yet): %+v", f)
		}
	}
}

func TestAPI055_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("writer.go", manualMutexAroundFileWriteSrc)
	findingByID(t, res, "API-055")

	after := review.GoSource("writer.go", noManualMutexAroundFileWriteSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("API-055 still fires after remediation to plain evo.File: %+v", f)
		}
	}
}

// API-056: a .After(...) edge kept only to avoid a race on shared/same-file
// state — both Tasks already claim that same resource (evo.File's own
// Path, or an EffectSpec Path/Resource claim), so Evo's
// automatic resource coordination (ZYS-840) already serializes them without
// the edge (ZYS-936).

const afterResourceContentionSameFileSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  // same file — avoid concurrent write race
  cacheTask.After(configTask)
}
`

func TestAPI056_AfterOnlyForSameFileContention_Fires(t *testing.T) {
	res := review.GoSource("run.go", afterResourceContentionSameFileSrc)
	f := findingByID(t, res, "API-056")
	if f.Severity != "warning" {
		t.Fatalf("API-056 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "delete") || !strings.Contains(f.Suggestion, "cacheTask.After(configTask)") {
		t.Fatalf("API-056 suggestion does not name the edge to delete: %q", f.Suggestion)
	}
	if !strings.Contains(f.Message, "config.json") {
		t.Fatalf("API-056 message does not name the shared resource: %q", f.Message)
	}
}

// afterResourceContentionEffectPathSrc: sweepTask and gcTask both DELETE
// the identical Path. Deleting an already-deleted path is idempotent,
// so which Task runs first never changes the outcome — a genuine
// order-invariant overlap, safe to report.
const afterResourceContentionEffectPathSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(sweepTask, gcTask *evo.Task) {
  sweepTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "repo cache", Path: "/repo/cache"}, removeRepoCache)
  })
  gcTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "repo cache", Path: "/repo/cache"}, removeRepoCache)
  })
  // shared resource, exclusive access only
  gcTask.After(sweepTask)
}
func removeRepoCache(context.Context) error { return nil }
`

func TestAPI056_AfterOnlyForEffectPathContention_Fires(t *testing.T) {
	res := review.GoSource("run.go", afterResourceContentionEffectPathSrc)
	findingByID(t, res, "API-056")
}

// afterResourceConflictingVerbsSrc: lockTask UPDATEs and sweepTask DELETEs
// the identical Path. Delete-then-update recreates the resource;
// update-then-delete removes it — the outcome genuinely depends on order,
// so this is a real dependency the rule must not tell the caller to delete.
const afterResourceConflictingVerbsSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(lockTask, sweepTask *evo.Task) {
  lockTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, Object: "repo", Path: "/repo"}, refreshRepo)
  })
  sweepTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "repo", Path: "/repo"}, removeRepo)
  })
  // shared resource, exclusive access only
  sweepTask.After(lockTask)
}
func refreshRepo(context.Context) error { return nil }
func removeRepo(context.Context) error  { return nil }
`

func TestAPI056_ConflictingVerbsEffectPath_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterResourceConflictingVerbsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 when conflicting Verbs (Update vs Delete) on the same resource make order a real dependency: %+v", f)
		}
	}
}

// afterResourceDifferingContentsSrc: configTask and cacheTask both write
// config.json, but with different Contents. Deleting the .After edge (this
// rule's own remediation) would leave config.json's final content
// nondeterministic across runs — a resource claim only coordinates the
// overlap, it does not decide which write wins.
const afterResourceDifferingContentsSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: warm})
  })
  // same file — avoid concurrent write race
  cacheTask.After(configTask)
}
`

func TestAPI056_DifferingFileContents_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterResourceDifferingContentsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 when the two File writes carry different Contents, so deleting .After leaves config.json nondeterministic: %+v", f)
		}
	}
}

const afterRealDependencySrc = `package p
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(prepareTask, buildTask *evo.Task) {
  prepareTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  buildTask.Define(func(ctx context.Context) error {
    _, err := os.ReadFile("config.json")
    return err
  })
  // buildTask needs the config prepareTask writes
  buildTask.After(prepareTask)
}
`

func TestAPI056_RealSemanticDependency_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterRealDependencySrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 on a real producer/consumer dependency: %+v", f)
		}
	}
}

const afterNoCommentSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.After(configTask)
}
`

func TestAPI056_NoContentionComment_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterNoCommentSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 with no comment naming exclusion as the reason: %+v", f)
		}
	}
}

func TestAPI056_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run.go", afterResourceContentionSameFileSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("API-056 fired for a pin older than 1.1.0 (automatic resource claims did not exist yet): %+v", f)
		}
	}
}

// afterResourceContentionRemediatedSrc is afterResourceContentionSameFileSrc
// with this rule's own Suggestion applied: cacheTask.After(configTask) and
// its comment are deleted, keeping only the overlapping File claim.
const afterResourceContentionRemediatedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
}
`

func TestAPI056_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("run.go", afterResourceContentionSameFileSrc)
	findingByID(t, res, "API-056")

	after := review.GoSource("run.go", afterResourceContentionRemediatedSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("API-056 still fires after applying its own prescribed remediation (deleting the .After(...) edge): %+v", f)
		}
	}
}

// A mutex that guards real shared state (a map) as well as the File call
// must never be deleted: the suggestion narrows the critical section
// instead, and never tells the agent to remove the lock or its field.
const mutexGuardingMapAndFileSrc = `package p
import (
  "context"
  "sync"
  eo "github.com/zachbornheimer/evident-output/v2"
)
type Cache struct {
  mu   sync.Mutex
  seen map[string]int
}
func (c *Cache) store(ctx context.Context, path string, contents []byte) error {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.seen[path]++
  return eo.File(ctx, eo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI055_MutexGuardingSharedStateAndFile_NeverSuggestsDelete(t *testing.T) {
	f := findingByID(t, review.GoSource("cache.go", mutexGuardingMapAndFileSrc), "API-055")
	if f.Severity != "warning" {
		t.Fatalf("API-055 severity = %q, want warning", f.Severity)
	}
	for _, banned := range []string{"delete", "remove", "field"} {
		if strings.Contains(strings.ToLower(f.Suggestion), banned) {
			t.Fatalf("API-055 suggestion would drop a lock that guards shared state (%q): %q", banned, f.Suggestion)
		}
	}
	if !strings.Contains(f.Suggestion, "c.mu.Unlock()") {
		t.Fatalf("API-055 suggestion does not name the lock to narrow: %q", f.Suggestion)
	}
}

// A lock-shaped call in a comment or string, or an evo.File mention in a
// string, is not code: the AST detector stays silent.
const lockAndFileOnlyInTextSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Doc struct {
  mu sync.Mutex
}
func (d *Doc) help(ctx context.Context) string {
  // d.mu.Lock() then evo.File(ctx, spec) was the old shape
  return "d.mu.Lock(); evo.File(ctx, evo.FileSpec{})"
}
`

func TestAPI055_LockAndFileOnlyInCommentsAndStrings_StaysSilent(t *testing.T) {
	assertNoRule(t, review.GoSource("doc.go", lockAndFileOnlyInTextSrc), "API-055")
}

// The File-only region keeps the removal suggestion but never mentions
// deleting the mutex field, which may guard other methods.
func TestAPI055_FileOnlyRegion_DoesNotSuggestDeletingField(t *testing.T) {
	f := findingByID(t, review.GoSource("writer.go", manualMutexAroundFileWriteSrc), "API-055")
	if strings.Contains(f.Suggestion, "field") {
		t.Fatalf("API-055 suggestion tells the agent to delete the mutex field: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "w.mu.Lock()/w.mu.Unlock()") {
		t.Fatalf("API-055 suggestion does not name the lock pair: %q", f.Suggestion)
	}
}

// A logical Resource name and a filesystem Path never overlap, even when
// their text is equal, so the edge between them is not redundant.
const afterPathVersusResourceNameSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(sweepTask, gcTask *evo.Task) {
  sweepTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "cache", Quantity: 1, Path: "cache"}, removeCache)
  })
  gcTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "cache", Quantity: 1, Resource: "cache"}, removeCache)
  })
  // shared resource, exclusive access only
  gcTask.After(sweepTask)
}
func removeCache(context.Context) error { return nil }
`

func TestAPI056_PathVersusResourceName_StaysSilent(t *testing.T) {
	assertNoRule(t, review.GoSource("run.go", afterPathVersusResourceNameSrc), "API-056")
}

func TestAPI053_NestedPatchInsideEffectClaim_Fires(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func moveTree(ctx context.Context, root string, diff []byte) error {
  spec := evo.EffectSpec{Object: "tree", Verb: evo.Update, Quantity: 1, Resource: "tree"}
  return evo.Effect(ctx, spec, func(ctx context.Context) error {
    return evo.Patch(ctx, diff)
  })
}
`
	findingByID(t, review.GoSource("tree.go", src), "API-053")
}
