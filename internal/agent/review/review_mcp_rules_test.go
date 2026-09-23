package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// PROG-001: the Advance suggestion must never prescribe <task>.Each(...) — it no longer exists (removed in 1.0; evo-dialect-axes-report.md axis 6/12).

func TestPROG001_AdvanceSuggestion_NeverNamesTaskEach(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Advance(1)
}
`
	res := review.GoSource("x.go", src)
	f := findingByID(t, res, "PROG-001")
	if strings.Contains(f.Suggestion, "task.Each") {
		t.Fatalf("PROG-001 suggestion invents task.Each: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "Group(") && !strings.Contains(f.Suggestion, "Sequence(") {
		t.Fatalf("PROG-001 suggestion does not name Group/Sequence.Each: %q", f.Suggestion)
	}
}

// FP-006: Doing(...).Done(...) with no Define between is theater.

const doingDoneChainSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func f(a *evo.Output, fixed int) {
  a.Task("file integrity").Doing("fixing").Done("%d files changed", fixed)
}
`

const doingDoneChainFixedSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func f(a *evo.Output) {
  t := a.Task("file integrity")
  t.Define(func(ctx context.Context) error {
    return nil
  })
}
`

func TestFP006_DoingDoneChain_Fires(t *testing.T) {
	res := review.GoSource("fix.go", doingDoneChainSrc)
	f := findingByID(t, res, "FP-006")
	if f.Severity != "error" {
		t.Fatalf("FP-006 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "Define") {
		t.Fatalf("FP-006 suggestion does not name Define: %q", f.Suggestion)
	}
}

func TestFP006_DefineInstead_StaysSilent(t *testing.T) {
	res := review.GoSource("fixed.go", doingDoneChainFixedSrc)
	for _, f := range res.Findings {
		if f.RuleID == "FP-006" {
			t.Fatalf("false positive FP-006 on Define: %+v", f)
		}
	}
}

const doingDoneAdjacentSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func f(t *evo.TaskHandle) {
  t.Doing("rechecking")
  applyFix()
  t.Done("deterministic fix applied")
}
func applyFix() {}
`

func TestFP006_DoingDoneAdjacentStatements_Fires(t *testing.T) {
	res := review.GoSource("fix.go", doingDoneAdjacentSrc)
	findingByID(t, res, "FP-006")
}

const doingProgressDoneSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func f(t *evo.TaskHandle) {
  t.Doing("scanning")
  t.Define(func(ctx context.Context) error {
    return nil
  })
  t.Done()
}
`

func TestFP006_DefineBetween_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", doingProgressDoneSrc)
	for _, f := range res.Findings {
		if f.RuleID == "FP-006" {
			t.Fatalf("false positive FP-006 when Define runs between Doing and Done: %+v", f)
		}
	}
}

// API-040: Failf/Fail inside a Define callback whose result is
// returned double-resolves the task (zq app.go:155-176 -> executeCommand).

const failfInDefineSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Define(func(ctx context.Context) error {
    if err := doWork(task); err != nil {
      return err
    }
    return nil
  })
}
func doWork(task *evo.TaskHandle) error {
  if err := resolve(); err != nil {
    return task.Failf("resolve: %w", err)
  }
  return nil
}
func resolve() error { return nil }
`

func TestAPI040_FailfReachableFromDefine_Fires(t *testing.T) {
	res := review.GoSource("app.go", failfInDefineSrc)
	f := findingByID(t, res, "API-040")
	if f.Severity != "error" {
		t.Fatalf("API-040 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "return err") {
		t.Fatalf("API-040 suggestion does not say return err: %q", f.Suggestion)
	}
}

const failThenReturnErrSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Define(func(ctx context.Context) error {
    err := doWork()
    if err != nil {
      task.Fail("resolve failed")
      return err
    }
    return nil
  })
}
func doWork() error { return nil }
`

func TestAPI040_FailThenReturnErr_Fires(t *testing.T) {
	res := review.GoSource("app.go", failThenReturnErrSrc)
	findingByID(t, res, "API-040")
}

const returnErrOnlySrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Define(func(ctx context.Context) error {
    if err := doWork(task); err != nil {
      return err
    }
    return nil
  })
}
func doWork(task *evo.TaskHandle) error {
  return resolve()
}
func resolve() error { return nil }
`

func TestAPI040_PlainReturnErr_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", returnErrOnlySrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-040" {
			t.Fatalf("false positive API-040 on plain return err: %+v", f)
		}
	}
}

// API-041: a goroutine resolving a predeclared Task with no Define inside.

const goroutinePredeclaredSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  t := out.Task("a")
  go func() {
    t.Doing("working")
    t.Done()
  }()
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  t := out.Task("a")
  go func() {
    t.Define(func(ctx context.Context) error { return nil })
  }()
}
`

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
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "module", Quantity: 2}, nil)
}
`

func TestAPI042_NilEffectCallback_Fires(t *testing.T) {
	res := review.GoSource("setup.go", nilEffectCallbackSrc)
	f := findingByID(t, res, "API-042")
	if f.Severity != "error" {
		t.Fatalf("API-042 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "Record") {
		t.Fatalf("API-042 suggestion does not name Record: %q", f.Suggestion)
	}
}

const noOpLiteralEffectCallbackSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktree", Quantity: 1}, func(context.Context) error { return nil })
}
`

func TestAPI042_NoOpLiteralEffectCallback_Fires(t *testing.T) {
	res := review.GoSource("clean.go", noOpLiteralEffectCallbackSrc)
	findingByID(t, res, "API-042")
}

const noOpNamedEffectCallbackSrc = `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, name string, n int) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "module", Quantity: n}, func(context.Context) error {
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
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "module", Quantity: 2}, func(ctx context.Context) error {
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
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktrees", Quantity: 1}, removeWorktree)
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
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktree", Quantity: 1}, removeWorktree)
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
import evo "github.com/zachbornheimer/evident-output"
type Row struct{ Object string }
func run(_ *evo.TaskHandle) Row { return Row{Object: "worktrees"} }
`

func TestAPI043_PluralNonEffectSpecObjectField_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", pluralNonEvoObjectFieldSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-043" {
			t.Fatalf("false positive API-043 on a non-EffectSpec Object field: %+v", f)
		}
	}
}

// API-044: a hand-rolled channel wrapper around Define reimplements Wait.

const channelWaitAroundDefineSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func defineAndWait(task *evo.TaskHandle, fn func() error) error {
  done := make(chan error, 1)
  task.Define(func(ctx context.Context) error {
    err := fn()
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
	if !strings.Contains(f.Suggestion, "task.Wait()") {
		t.Fatalf("API-044 suggestion does not name task.Wait(): %q", f.Suggestion)
	}
}

const plainDefineNoChannelSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle, fn func() error) {
  task.Define(fn)
}
`

func TestAPI044_PlainDefine_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", plainDefineNoChannelSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-044" {
			t.Fatalf("false positive API-044 on a plain Define: %+v", f)
		}
	}
}

// TAX-003: inline evo.Reason literal, and a reason that restates its verb.

const inlineReasonSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("timeout"))
}
`

func TestTAX003_InlineReason_FiresConstantsSuggestion(t *testing.T) {
	res := review.GoSource("run.go", inlineReasonSrc)
	f := findingByID(t, res, "TAX-003")
	if f.Severity != "warning" {
		t.Fatalf("TAX-003 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "var reason") {
		t.Fatalf("TAX-003 suggestion does not offer a package-level var: %q", f.Suggestion)
	}
}

const reasonRestatesVerbSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("skipped"))
}
`

func TestTAX003_ReasonRestatesVerb_Fires(t *testing.T) {
	res := review.GoSource("run.go", reasonRestatesVerbSrc)
	f := findingByID(t, res, "TAX-003")
	if !strings.Contains(f.Message, "restates") {
		t.Fatalf("TAX-003 message does not call out the restated verb: %q", f.Message)
	}
}

const reasonVarBlockSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
var reasonProtected = evo.Reason("protected")
func run(task *evo.TaskHandle) {
  task.Skipped(reasonProtected)
}
`

func TestTAX003_VarBlockDeclaration_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", reasonVarBlockSrc)
	for _, f := range res.Findings {
		if f.RuleID == "TAX-003" {
			t.Fatalf("false positive TAX-003 on a var-block Reason declaration: %+v", f)
		}
	}
}

func TestTAX003_TestFile_StaysSilent(t *testing.T) {
	res := review.GoSource("run_test.go", inlineReasonSrc)
	for _, f := range res.Findings {
		if f.RuleID == "TAX-003" {
			t.Fatalf("false positive TAX-003 in a _test.go file: %+v", f)
		}
	}
}

// API-046: task.Skipped(evo.Reason("...")) whose reason names an
// already-satisfied condition instead of true inapplicability.

const skippedAlreadyUpToDateSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle, installed, latest string) {
  if installed == latest {
    task.Skipped(evo.Reason("already up to date"))
    return
  }
  task.Define(func(ctx context.Context) error { return nil })
}
`

func TestAPI046_SkippedAlreadyUpToDate_Fires(t *testing.T) {
	res := review.GoSource("adopt.go", skippedAlreadyUpToDateSrc)
	f := findingByID(t, res, "API-046")
	if f.Severity != "warning" {
		t.Fatalf("API-046 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "Verify") {
		t.Fatalf("API-046 suggestion does not name Verify: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "ResolutionAlreadySatisfied") {
		t.Fatalf("API-046 suggestion does not name ResolutionAlreadySatisfied: %q", f.Suggestion)
	}
}

func TestAPI046_SkippedBareWordReasons_Fire(t *testing.T) {
	cases := []string{"already latest", "unchanged", "current", "latest", "up to date"}
	for _, reason := range cases {
		src := `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("` + reason + `"))
}
`
		t.Run(reason, func(t *testing.T) {
			res := review.GoSource("adopt.go", src)
			findingByID(t, res, "API-046")
		})
	}
}

const skippedNoGoModuleSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("no Go module"))
}
`

func TestAPI046_TrueInapplicability_StaysSilent(t *testing.T) {
	res := review.GoSource("adopt.go", skippedNoGoModuleSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-046" {
			t.Fatalf("false positive API-046 on true inapplicability: %+v", f)
		}
	}
}

const skippedUnrelatedReasonSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("protected"))
}
`

func TestAPI046_UnrelatedReason_StaysSilent(t *testing.T) {
	res := review.GoSource("adopt.go", skippedUnrelatedReasonSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-046" {
			t.Fatalf("false positive API-046 on an unrelated skip reason: %+v", f)
		}
	}
}

func TestAPI046_PreOneZeroPin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("adopt.go", skippedAlreadyUpToDateSrc, "0.6.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-046" {
			t.Fatalf("false positive API-046 for a pre-1.0.0 pin: %+v", f)
		}
	}
}

func TestAPI046_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("adopt.go", skippedAlreadyUpToDateSrc)
	findingByID(t, res, "API-046")

	const remediated = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle, installed, latest string) {
  task.Verify(func(ctx context.Context) (bool, error) {
    return installed == latest, nil
  })
  task.Define(func(ctx context.Context) error { return nil })
}
`
	after := review.GoSource("adopt.go", remediated)
	for _, f := range after.Findings {
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("file integrity").Done()
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("fix").Done()
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output, g *evo.GroupHandle) {
  out.Task("check file integrity").Done()
  g.Task("format Python").Done()
  g.Task("stabilize Go source").Done()
  g.Task("lint Go").Done()
  g.Task("check Python").Done()
  g.Task("build application icons").Done()
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
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Fact("merge markers", "none found")
  task.Fact("symlinks", "valid")
  task.Warn("generated file looks stale")
}
`
	res := review.GoSource("integrity.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-045" {
			t.Fatalf("false positive API-045 on Facts/Warn under one Task: %+v", f)
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("fix").Define(func(ctx context.Context) error {
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  t := out.Task("check")
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("check").Define(func(ctx context.Context) error {
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("stabilize Go source").Define(func(ctx context.Context) error {
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  fixGroup := out.Group("fix")
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

// API-047: a Task/Group/Sequence declaration reuses a sibling literal name
// already used by a different entity kind under the same parent (ZYS-944).
// Same-kind reuse already fails fast at runtime
// (engine.ProblemCodeDuplicateSiblingName); this rule closes the cross-kind
// gap statically.

const crossKindDuplicateSiblingSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("build")
  out.Group("build")
}
`

func TestAPI047_CrossKindDuplicateSiblingName_Fires(t *testing.T) {
	res := review.GoSource("run.go", crossKindDuplicateSiblingSrc)
	f := findingByID(t, res, "API-047")
	if f.Severity != "error" {
		t.Fatalf("API-047 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "out.Group(") {
		t.Fatalf("API-047 suggestion does not name the corrected out.Group(...) call: %q", f.Suggestion)
	}
	if !strings.Contains(f.Message, "task") || !strings.Contains(f.Message, "group") {
		t.Fatalf("API-047 message does not name both conflicting kinds: %q", f.Message)
	}
}

const crossKindDuplicateSiblingNestedSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  work := out.Group("work")
  work.Task("build")
  work.Sequence("build")
}
`

func TestAPI047_CrossKindDuplicateSiblingName_FiresUnderNestedParent(t *testing.T) {
	res := review.GoSource("run.go", crossKindDuplicateSiblingNestedSrc)
	f := findingByID(t, res, "API-047")
	if !strings.Contains(f.Suggestion, "work.Sequence(") {
		t.Fatalf("API-047 suggestion does not name the corrected work.Sequence(...) call: %q", f.Suggestion)
	}
}

const sameKindDuplicateSiblingSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("build")
  out.Task("build")
}
`

func TestAPI047_SameKindDuplicate_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", sameKindDuplicateSiblingSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("API-047 fired for a same-kind duplicate; the runtime's own duplicate-sibling-name failure already covers this: %+v", f)
		}
	}
}

const distinctSiblingNamesSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  out.Task("build")
  out.Group("test")
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
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  a := out.Group("a")
  b := out.Group("b")
  a.Task("build")
  b.Group("build")
}
`

func TestAPI047_CrossKindDifferentParents_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", crossKindDifferentParentsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("false positive API-047 across two distinct parents that merely share a literal child name: %+v", f)
		}
	}
}

const crossKindDuplicateSiblingBranchesSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output, cond bool) {
  if cond {
    out.Task("build")
  } else {
    out.Group("build")
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

func TestAPI047_PreOneZeroPin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run.go", crossKindDuplicateSiblingSrc, "0.6.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-047" {
			t.Fatalf("API-047 fired for a pin older than 1.0.0: %+v", f)
		}
	}
}

// API-048: a Group/Sequence Task re-declared by the same string literal to
// obtain a later dependency reference — declareGroupTask fails the second
// call as a duplicate sibling rather than returning the first handle (§3.1;
// internal/engine/group.go's GroupHandle.Task doc comment), so the fix is a
// typed variable (or var (...) block) holding the one handle, per the
// product contract's own zq prune example.

const redeclaredTaskForAfterSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(prune *evo.GroupHandle) {
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
import evo "github.com/zachbornheimer/evident-output"
func run(prune *evo.GroupHandle) {
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
import evo "github.com/zachbornheimer/evident-output"
func run(prune *evo.GroupHandle) {
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
import evo "github.com/zachbornheimer/evident-output"
func run(a, b *evo.GroupHandle) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, task *evo.TaskHandle) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, task *evo.TaskHandle) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, task *evo.TaskHandle) {
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
// TaskHandle.Define — ZYS-938's proven false positive.
const defineOnUnrelatedTypeSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)

type Validator struct{}

func (v *Validator) Define(fn func(context.Context) error) {
  _ = fn
}

func run(ctx context.Context, task *evo.TaskHandle) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, task *evo.TaskHandle) {
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
// with TaskHandle.Problem (ZYS-848/ZYS-943, docs/migration/1.1.md
// "TaskHandle.Problem — a Task can now own many blocking findings").

const flattenedDiagnosticsLoopSrc = `package p
import (
  "errors"
  "strings"

  evo "github.com/zachbornheimer/evident-output"
)
func blockStagedGolangciFindings(task *evo.TaskHandle, findings []finding) error {
  var lines []string
  for _, f := range findings {
    lines = append(lines, formatFinding(f))
  }
  return errors.New(strings.Join(lines, "\n"))
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
import evo "github.com/zachbornheimer/evident-output"
func reportFileIntegrityIssues(group *evo.GroupHandle, findings []finding) {
  for _, f := range findings {
    group.Task(f.File).Fail(f.Message)
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output, issues []issue) {
  task := out.Task("file integrity")
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
			t.Fatalf("API-051 fired for a pin older than 1.1.0 (TaskHandle.Problem accumulation did not exist yet): %+v", f)
		}
	}
	res = review.GoSourceAt("hook.go", fakeTaskPerFindingSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-051" {
			t.Fatalf("API-051 fired for a pin older than 1.1.0 (TaskHandle.Problem accumulation did not exist yet): %+v", f)
		}
	}
}

// API-052: a caller stores Group/Sequence child Task handles solely to loop
// Wait, filter ErrNotStarted, Snapshot the container, count failed children,
// and synthesize its own aggregate error — GroupHandle.Wait/SequenceHandle.Wait
// (ZYS-849) now owns exactly this (ZYS-941, zq runParallel/
// waitDefinedRunOperations evidence).

const callerWaitLoopSnapshotCountSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func runParallel(jobs *evo.GroupHandle, items []string) error {
  var handles []*evo.TaskHandle
  for _, item := range items {
    t := jobs.Task(item)
    t.Define(func(ctx context.Context) error { return nil })
    handles = append(handles, t)
  }
  failed := 0
  for _, h := range handles {
    if err := h.Wait(); err != nil {
      failed++
    }
  }
  snap := jobs.Snapshot()
  _ = snap
  if failed > 0 {
    return fmt.Errorf("%d of %d failed", failed, len(handles))
  }
  return nil
}
`

func TestAPI052_CallerWaitLoopSnapshotCount_Fires(t *testing.T) {
	res := review.GoSource("run_parallel.go", callerWaitLoopSnapshotCountSrc)
	f := findingByID(t, res, "API-052")
	if f.Severity != "error" {
		t.Fatalf("API-052 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "GroupHandle.Wait") && !strings.Contains(f.Suggestion, "jobs.Wait()") {
		t.Fatalf("API-052 suggestion does not name the container Wait fix: %q", f.Suggestion)
	}
}

const callerWaitLoopErrNotStartedSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func waitDefinedRunOperations(tasks []*evo.TaskHandle) error {
  for _, task := range tasks {
    err := task.Wait()
    if errors.Is(err, evo.ErrNotStarted) {
      continue
    }
    if err != nil {
      return err
    }
  }
  return nil
}
`

func TestAPI052_CallerWaitLoopErrNotStartedFilter_Fires(t *testing.T) {
	res := review.GoSource("run_execute.go", callerWaitLoopErrNotStartedSrc)
	f := findingByID(t, res, "API-052")
	if !strings.Contains(f.Suggestion, "SequenceHandle.Wait") && !strings.Contains(f.Suggestion, "GroupHandle.Wait") {
		t.Fatalf("API-052 suggestion does not name the container Wait surface: %q", f.Suggestion)
	}
}

const containerWaitDirectSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func runParallel(jobs *evo.GroupHandle, items []string) error {
  for _, item := range items {
    jobs.Task(item).Define(func(ctx context.Context) error { return nil })
  }
  return jobs.Wait()
}
`

func TestAPI052_ContainerWaitDirectly_StaysSilent(t *testing.T) {
	res := review.GoSource("run_parallel.go", containerWaitDirectSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-052" {
			t.Fatalf("false positive API-052 when the caller already uses container.Wait(): %+v", f)
		}
	}
}

const unrelatedForLoopWaitSrc = `package p
import "os/exec"
func runAll(cmds []*exec.Cmd) error {
  for _, c := range cmds {
    if err := c.Wait(); err != nil {
      return err
    }
  }
  return nil
}
`

func TestAPI052_UnrelatedExecCmdWaitLoop_StaysSilent(t *testing.T) {
	res := review.GoSource("run_cmds.go", unrelatedForLoopWaitSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-052" {
			t.Fatalf("false positive API-052 on an unrelated os/exec Cmd.Wait() loop: %+v", f)
		}
	}
}

func TestAPI052_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run_parallel.go", callerWaitLoopSnapshotCountSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-052" {
			t.Fatalf("API-052 fired for a pin older than 1.1.0 (container Wait did not exist yet): %+v", f)
		}
	}
}

func TestAPI052_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("run_parallel.go", callerWaitLoopSnapshotCountSrc)
	findingByID(t, res, "API-052")

	after := review.GoSource("run_parallel.go", containerWaitDirectSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-052" {
			t.Fatalf("API-052 still fires after remediation to container.Wait(): %+v", f)
		}
	}
}

// API-053: nested Evo resource acquisition (ZYS-933/ZYS-840).

const nestedResourceDirectSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
  spec := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(from)}
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
  evo "github.com/zachbornheimer/evident-output"
)
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
  spec := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(from)}
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
  evo "github.com/zachbornheimer/evident-output"
)
func moveTwoWorktrees(ctx context.Context, a, b string) error {
  outer := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(a)}
  return evo.Effect(ctx, outer, func(ctx context.Context) error {
    inner := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(b)}
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
  evo "github.com/zachbornheimer/evident-output"
)
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
  if err := evo.File(ctx, evo.FileSpec{Path: to, Contents: marker}); err != nil {
    return err
  }
  spec := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(from)}
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
  evo "github.com/zachbornheimer/evident-output"
)
func cleanCache(ctx context.Context, path string, marker []byte) error {
  spec := evo.EffectSpec{Object: "cache entry", Verb: evo.EffectDelete}
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
  evo "github.com/zachbornheimer/evident-output"
)
func runCapturedTask(task *evo.TaskHandle, cmd *exec.Cmd) error {
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
  evo "github.com/zachbornheimer/evident-output"
)
func runChecked(task *evo.TaskHandle, cmd *exec.Cmd) error {
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
import evo "github.com/zachbornheimer/evident-output"
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
  evo "github.com/zachbornheimer/evident-output"
)
func inspectExecResult(task *evo.TaskHandle, res evo.ExecResult) error {
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
