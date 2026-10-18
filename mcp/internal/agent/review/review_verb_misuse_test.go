package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

func TestAPI033_NameEqualsExcludeArgument(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, note string) {
  r.Task(note).Define(func(context.Context) error { return evo.Exclude(note) })
}
`
	res := review.GoSource("dup.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-033" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-033 for Task(note) returning Exclude(note): %+v", res.Findings)
	}
}

func TestAPI033_NoFalsePositiveOnDistinctNameAndReason(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, reason string) {
  r.Task("branch check").Define(func(context.Context) error { return evo.Exclude(reason) })
}
`
	res := review.GoSource("distinct.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-033" {
			t.Fatalf("false positive API-033 on distinct name/reason: %+v", res.Findings)
		}
	}
}

func TestAPI035_DiscardSinkInFailingBlock(t *testing.T) {
	src := `package p
import (
  "io"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(cmd *exec.Cmd) error {
  cmd.Stdout = io.Discard
  if err := cmd.Run(); err != nil {
    return evo.Refuse("policy check failed")
  }
  return nil
}
`
	res := review.GoSource("discard.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-035" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-035: %+v", res.Findings)
	}
}

func TestAPI035_NoFalsePositiveWithoutFailRefuse(t *testing.T) {
	src := `package p
import (
  "io"
  "os/exec"
)
func f(cmd *exec.Cmd) {
  cmd.Stdout = io.Discard
  _ = cmd.Run()
}
`
	res := review.GoSource("discardclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-035" {
			t.Fatalf("false positive API-035 with no Fail/Refuse in the function: %+v", res.Findings)
		}
	}
}

// TestAPI038_SprintfIntoVariadicVerb is red-first for the new detector: a
// printf-variadic evo method (Doing here) already accepts format + args
// directly, so wrapping the call in fmt.Sprintf is ceremony to flatten away.
func TestAPI038_SprintfIntoVariadicVerb(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(task *evo.Task, path string) {
  task.Doing(fmt.Sprintf("scanning %s", path))
}
`
	res := review.GoSource("sprintfdoing.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-038" {
			found = true
			if f.Suggestion != `task.Doing("scanning %s", path)` {
				t.Fatalf("suggestion = %q", f.Suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("expected API-038: %+v", res.Findings)
	}
}

// TestAPI038_WarnMigratesToProblem proves dirty task.Warn(fmt.Sprintf(...))
// rewrites through API-032 to Task.Problem. Warn is removed
// in 1.1: do not flatten the Sprintf into task.Warn, and do not invent Warnf.
func TestAPI038_WarnMigratesToProblem(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(task *evo.TaskHandle, n int) {
  task.Warn(fmt.Sprintf("kept %d", n))
}
`
	res := review.GoSource("sprintfwarn.go", src)
	var api032 review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-038" {
			t.Fatalf("API-038 must not flatten Warn; migrate to Problem: %+v", f)
		}
		if f.RuleID == "API-032" && strings.Contains(f.Suggestion, "Problem(") {
			api032 = f
		}
	}
	if api032.Suggestion == "" {
		t.Fatalf("expected API-032 Problem rewrite, got %+v", res.Findings)
	}
	if !strings.Contains(api032.Suggestion, `task.Problem(fmt.Sprintf("kept %d", n))`) {
		t.Fatalf("suggestion = %q", api032.Suggestion)
	}
}

func TestAPI038_NoFalsePositiveWithoutSprintf(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.Task, done, total int) {
  task.Doing("scanning %d/%d", done, total)
}
`
	res := review.GoSource("doingclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-038" {
			t.Fatalf("false positive API-038 on an already-flattened call: %+v", res.Findings)
		}
	}
}

func TestAPI037_WrapperMethodOverTaskVerb(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type runner struct{ task *evo.Task }
func (r *runner) resolutionPhase(text string) {
  r.task.Doing(text)
}
`
	res := review.GoSource("wrapper.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-037: %+v", res.Findings)
	}
}

func TestAPI037_NoFalsePositiveOnMultiStatementMethod(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type runner struct{ task *evo.Task }
func (r *runner) resolutionPhase(text string) {
  r.task.Doing(text)
  r.task.Progress(1, 2)
}
`
	res := review.GoSource("wrapperclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			t.Fatalf("false positive API-037 on a multi-statement method: %+v", res.Findings)
		}
	}
}

// A wrapper that composes the verb's argument is not a bare passthrough:
// the caller cannot "inline the verb" without copying that composition, so
// the name and the frame are earning their place.
func TestAPI037_NoFalsePositiveOnComposedArgument(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type subject struct{ classify *evo.Task }
func (s subject) Problem(summary string, cause error) {
  s.classify.Problem(fmt.Sprintf("%s: %s", summary, cause))
}
`
	res := review.GoSource("composed.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			t.Fatalf("false positive API-037 on a composed argument: %+v", res.Findings)
		}
	}
}

func TestDOM018_ErrTwiceViaDetail(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(err error) error {
  return evo.Fail(err.Error(), evo.Detail(err.Error()))
}
`
	res := review.GoSource("errtwice.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "DOM-018" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DOM-018: %+v", res.Findings)
	}
}

func TestDOM018_NoFalsePositiveOnDistinctErrorVars(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(err, other error) error {
  return evo.Fail(err.Error(), evo.Detail(other.Error()))
}
`
	res := review.GoSource("errdistinct.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-018" {
			t.Fatalf("false positive DOM-018 on distinct error variables: %+v", res.Findings)
		}
	}
}

func TestTAX002_DynamicReasonFromJoin(t *testing.T) {
	src := `package p
import (
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(names []string) error {
  return evo.Exclude(strings.Join(names, ", "))
}
`
	res := review.GoSource("dynreason.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TAX-002" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TAX-002: %+v", res.Findings)
	}
}

func TestTAX002_NoFalsePositiveOnLiteralOrPackageVar(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
const reasonProtected = "protected"
func f(literal bool) error {
  if literal {
    return evo.Exclude("protected")
  }
  return evo.Exclude(reasonProtected)
}
`
	res := review.GoSource("reasonclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "TAX-002" {
			t.Fatalf("false positive TAX-002 on a literal/package-var reason: %+v", res.Findings)
		}
	}
}

func TestTXT020_LongEntityName(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("copying build artifacts into the production release bucket now")
}
`
	res := review.GoSource("longname.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TXT-020" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TXT-020: %+v", res.Findings)
	}
}

func TestTXT020_NoFalsePositiveOnShortNoun(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("release artifacts")
}
`
	res := review.GoSource("shortname.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "TXT-020" {
			t.Fatalf("false positive TXT-020 on a short noun name: %+v", res.Findings)
		}
	}
}

func TestDOM019_ShadowedHandleBeforeResolution(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  t := r.Task("scan")
  t.Doing("walking")
  t = r.Task("build")
  t.Define(build)
}
`
	res := review.GoSource("shadow.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "DOM-019" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DOM-019: %+v", res.Findings)
	}
}

func TestDOM019_NoFalsePositiveWhenDefinedFirst(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run, flag bool) {
  t := r.Task("scan")
  t.Doing("walking")
  t.Define(scan)
  t = r.Task("build")
  t.Define(build)
  if flag {
    t := r.Task("publish")
    t.Define(publish)
  }
}
`
	res := review.GoSource("shadowclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-019" {
			t.Fatalf("false positive DOM-019 when the first Task was Defined before reassignment: %+v", res.Findings)
		}
	}
}

func TestTXT021_CrammedSummary(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("policy").Define(func(ctx context.Context) error {
    return evo.Fail("policy check failed — cause: manifest missing — action: run zq init")
  })
}
`
	res := review.GoSource("crammed.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TXT-021" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TXT-021: %+v", res.Findings)
	}
}

func TestTXT021_NoFalsePositiveOnPlainSummary(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("policy").Define(func(ctx context.Context) error {
    return evo.Fail("policy check failed", evo.Detail("manifest missing"))
  })
}
`
	res := review.GoSource("plainsummary.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "TXT-021" {
			t.Fatalf("false positive TXT-021 on a plain summary: %+v", res.Findings)
		}
	}
}

// TestDOM020_GuidanceOnlyResolvesButIsNeverEmitted pins the exit-code
// honesty rule as guidance-only: rules.Explain resolves it (so a guide
// reference is never a dead end), but no fixture can make review.GoSource
// emit it — telling a usage mistake from a genuine evaluation failure needs
// the caller's own domain judgment, not a source pattern.
func TestDOM020_GuidanceOnlyResolvesButIsNeverEmitted(t *testing.T) {
	r, ok := rules.Explain("DOM-020")
	if !ok {
		t.Fatal("rules.Explain(\"DOM-020\") did not resolve")
	}
	if r.Detection != "guidance" {
		t.Fatalf("Detection = %q, want %q", r.Detection, "guidance")
	}
}

func TestLOOP001_SilentForRangeBeforeTask(t *testing.T) {
	bad := `package app
import (
  "context"
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run, roots []string) error {
  for _, root := range roots {
    _ = filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil })
  }
  r.Task("inventory").Define(func(ctx context.Context) error { return nil })
  return nil
}
`
	res := review.GoSource("purge_loop.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "LOOP-001" {
			found = true
			if f.Line == 0 {
				t.Error("LOOP-001 missing line")
			}
			if !strings.Contains(f.Suggestion, "Define") {
				t.Fatalf("LOOP-001 suggestion must name Define, got %q", f.Suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("expected LOOP-001 on for/range WalkDir before Task: %+v", res.Findings)
	}
}

func TestLOOP001_NoFalsePositiveWhenLoopInsideDefine(t *testing.T) {
	good := `package app
import (
  "context"
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run, roots []string) error {
  inv := r.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func(ctx context.Context) error {
    for _, root := range roots {
      if err := filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil }); err != nil {
        return err
      }
    }
    return nil
  })
  return nil
}
`
	res := review.GoSource("purge_loop_ok.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "LOOP-001" || f.RuleID == "FP-002" {
			t.Fatalf("false positive %s when Task/Doing/Define runs before the loop: %+v", f.RuleID, res.Findings)
		}
	}
}

func TestLOOP001_NoFalsePositiveOnGroupTaskPerItem(t *testing.T) {
	good := `package app
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func scan(ctx context.Context, r *evo.Run, paths []string) {
  worktrees := r.Group("worktrees")
  for _, path := range paths {
    path := path
    worktrees.Task(path).Define(func(ctx context.Context) error { return filepath.WalkDir(path, nil) })
  }
}
`
	res := review.GoSource("group_task_ok.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "LOOP-001" {
			t.Fatalf("false positive LOOP-001 on Group + one Task per item: %+v", res.Findings)
		}
	}
}
