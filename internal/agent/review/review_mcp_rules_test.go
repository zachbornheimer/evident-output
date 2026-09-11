package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// PROG-001: the Advance suggestion must never prescribe a nonexistent
// <task>.Each(...) (evo-dialect-axes-report.md axis 6/12).

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

// FP-006: Doing(...).Done(...) with no Define/verb between is theater.

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
  t.Define(func() error {
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
  t.Define(func() error {
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

// API-040: Failf/Fail inside a Define/mutation callback whose result is
// returned double-resolves the task (zq app.go:155-176 -> executeCommand).

const failfInDefineSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Define(func() error {
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
  task.Define(func() error {
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
  task.Define(func() error {
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
		t.Fatalf("API-041 suggestion does not name Group.Each/Define: %q", f.Suggestion)
	}
}

const goroutineWithDefineSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(out *evo.Output) {
  t := out.Task("a")
  go func() {
    t.Define(func() error { return nil })
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

// API-042: mutation verb with a nil or no-op callback.

const nilMutationCallbackSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Create("module", nil, evo.Affected(2))
}
`

func TestAPI042_NilCallback_Fires(t *testing.T) {
	res := review.GoSource("setup.go", nilMutationCallbackSrc)
	f := findingByID(t, res, "API-042")
	if f.Severity != "error" {
		t.Fatalf("API-042 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "Record") {
		t.Fatalf("API-042 suggestion does not name Record: %q", f.Suggestion)
	}
}

const noOpNamedCallbackSrc = `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, name string, n int) {
  task.Create("module", func() error {
    return installedCount(name, n)
  }, evo.Affected(n))
}
func installedCount(name string, count int) error {
  if name == "" || count < 1 {
    return fmt.Errorf("expected installed modules")
  }
  return nil
}
`

func TestAPI042_NoOpDelegatedCallback_Fires(t *testing.T) {
	res := review.GoSource("setup.go", noOpNamedCallbackSrc)
	findingByID(t, res, "API-042")
}

const realMutationCallbackSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Create("module", func() error {
    return installPackages()
  }, evo.Affected(2))
}
func installPackages() error { return invokeUV() }
`

func TestAPI042_RealWorkCallback_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", realMutationCallbackSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-042" {
			t.Fatalf("false positive API-042 on a callback that calls real work: %+v", f)
		}
	}
}

// API-043: a plural object literal on a mutation verb.

const pluralMutationObjectSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Delete("worktrees", func() error { return nil }, evo.Affected(1))
}
`

func TestAPI043_PluralObjectLiteral_Fires(t *testing.T) {
	res := review.GoSource("clean.go", pluralMutationObjectSrc)
	f := findingByID(t, res, "API-043")
	if f.Severity != "warning" {
		t.Fatalf("API-043 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, `"worktree"`) {
		t.Fatalf("API-043 suggestion does not name the singular: %q", f.Suggestion)
	}
}

const singularMutationObjectSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle) {
  task.Delete("worktree", func() error { return nil }, evo.Affected(1))
}
`

func TestAPI043_SingularObjectLiteral_StaysSilent(t *testing.T) {
	res := review.GoSource("good.go", singularMutationObjectSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-043" {
			t.Fatalf("false positive API-043 on singular object: %+v", f)
		}
	}
}

// API-044: a hand-rolled channel wrapper around Define reimplements Wait.

const channelWaitAroundDefineSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func defineAndWait(task *evo.TaskHandle, fn func() error) error {
  done := make(chan error, 1)
  task.Define(func() error {
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
