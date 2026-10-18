package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

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
