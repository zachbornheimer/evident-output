package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

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
