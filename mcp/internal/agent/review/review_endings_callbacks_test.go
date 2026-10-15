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
