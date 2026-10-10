package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// API-058 (ZYS-934): a patch is applied straight to the real workspace
// through os/exec ("patch", "git apply", "git am") instead of deriving
// desired file states with evo.Patch and committing them through
// evo.Files/evo.File.

const execPatchCommandSrc = `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, diffPath string) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "workspace", Quantity: 1}, func(context.Context) error {
    cmd := exec.Command("patch", "-p1", "-i", diffPath)
    return cmd.Run()
  })
}
`

func TestAPI058_ExecPatchCommand_Fires(t *testing.T) {
	res := review.GoSource("apply.go", execPatchCommandSrc)
	f := findingByID(t, res, "API-058")
	if f.Severity != "error" {
		t.Fatalf("API-058 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.Patch") || !strings.Contains(f.Suggestion, "evo.Files") {
		t.Fatalf("API-058 suggestion does not name evo.Patch/evo.Files: %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-058 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

const execGitApplyCommandSrc = `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func run(diffPath string) error {
  _ = evo.Config{}
  cmd := exec.Command("git", "apply", "--index", diffPath)
  return cmd.Run()
}
`

func TestAPI058_ExecGitApplyCommand_Fires(t *testing.T) {
	res := review.GoSource("apply_git.go", execGitApplyCommandSrc)
	findingByID(t, res, "API-058")
}

const execGitAmCommandSrc = `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func run(mboxPath string) error {
  _ = evo.Config{}
  cmd := exec.Command("git", "am", mboxPath)
  return cmd.Run()
}
`

func TestAPI058_ExecGitAmCommand_Fires(t *testing.T) {
	res := review.GoSource("apply_am.go", execGitAmCommandSrc)
	findingByID(t, res, "API-058")
}

// A patch binary invoked by full path is still a patch-apply mutation —
// detection matches on the executable's base name, not the literal string.

const execPatchFullPathSrc = `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func run(diffPath string) error {
  _ = evo.Config{}
  cmd := exec.Command("/usr/bin/patch", "-p0", diffPath)
  return cmd.Run()
}
`

func TestAPI058_ExecPatchFullPath_Fires(t *testing.T) {
	res := review.GoSource("apply_fullpath.go", execPatchFullPathSrc)
	findingByID(t, res, "API-058")
}

// Domain code that only parses a patch — no exec, no direct os-level
// mutation — must stay silent; evo.Patch itself is exactly this shape.

const domainPatchParseOnlySrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context, diff []byte) error {
  files, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.Files(ctx, files)
}
`

func TestAPI058_DomainPatchParseOnly_StaysSilent(t *testing.T) {
	res := review.GoSource("apply_good.go", domainPatchParseOnlySrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("false positive API-058 on domain code that only parses a patch: %+v", f)
		}
	}
}

// A "git" exec call that isn't apply/am (e.g. git status) is not a patch
// application and must stay silent.

const execGitStatusSrc = `package p
import "os/exec"
func run() error {
  cmd := exec.Command("git", "status", "--short")
  return cmd.Run()
}
`

func TestAPI058_ExecGitStatus_StaysSilent(t *testing.T) {
	res := review.GoSource("status.go", execGitStatusSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("false positive API-058 on exec.Command(\"git\", \"status\", ...): %+v", f)
		}
	}
}

// An unrelated exec.Command invocation must stay silent.

const execUnrelatedCommandSrc = `package p
import "os/exec"
func run() error {
  cmd := exec.Command("ls", "-la")
  return cmd.Run()
}
`

func TestAPI058_ExecUnrelatedCommand_StaysSilent(t *testing.T) {
	res := review.GoSource("list.go", execUnrelatedCommandSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("false positive API-058 on an unrelated exec.Command: %+v", f)
		}
	}
}

func TestAPI058_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("apply.go", execPatchCommandSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("API-058 fired for a pin older than 1.1.0 (evo.Patch/evo.Files did not exist yet): %+v", f)
		}
	}
}

func TestAPI058_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("apply.go", execPatchCommandSrc)
	findingByID(t, res, "API-058")

	after := review.GoSource("apply_good.go", domainPatchParseOnlySrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("API-058 still fires after remediation to evo.Patch/evo.Files: %+v", f)
		}
	}
}
