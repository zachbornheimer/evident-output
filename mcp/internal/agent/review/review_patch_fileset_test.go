package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/mcp/internal/agent/review"
)

// API-059 (ZYS-935): a Patch-derived FileSet is never passed to evo.Files —
// instead the same function reconstructs a fresh evo.FileSpec and commits
// through evo.File, discarding the Basis/stale-write guard the FileSet
// carried.

// patchFileSetPin is the newest pin whose evo.Patch returns a FileSet;
// API-059 applies only below 2.0.0.
const patchFileSetPin = "1.9.0"

const patchFileSetDiscardedNamedVarSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, diff []byte, path string) error {
  fileSet, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  _ = fileSet
  existing, err := os.ReadFile(path)
  if err != nil {
    return err
  }
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: existing})
}
`

func TestAPI059_PatchFileSetDiscardedNamedVar_Fires(t *testing.T) {
	res := review.GoSourceAt("apply.go", patchFileSetDiscardedNamedVarSrc, patchFileSetPin)
	f := findingByID(t, res, "API-059")
	if f.Severity != "error" {
		t.Fatalf("API-059 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.Files(ctx, fileSet)") {
		t.Fatalf("API-059 suggestion does not name evo.Files(ctx, fileSet): %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-059 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// The FileSet return value can be discarded outright with "_" instead of
// merely going unused — just as much a strip of the Basis/stale-write
// guard.

const patchFileSetDiscardedBlankSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, diff []byte, path string, contents []byte) error {
  _, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI059_PatchFileSetDiscardedBlank_Fires(t *testing.T) {
	res := review.GoSourceAt("apply_blank.go", patchFileSetDiscardedBlankSrc, patchFileSetPin)
	findingByID(t, res, "API-059")
}

// evo.Files(ctx, fileSet) is the remediated shape and must stay silent.

const patchFilesGoodCodeSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context, diff []byte) error {
  fileSet, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.Files(ctx, fileSet)
}
`

func TestAPI059_EvoFilesGoodCode_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("apply_good.go", patchFilesGoodCodeSrc, patchFileSetPin)
	for _, f := range res.Findings {
		if f.RuleID == "API-059" {
			t.Fatalf("false positive API-059 on evo.Files(ctx, fileSet): %+v", f)
		}
	}
}

// A plain evo.File write with no Patch call in scope at all is ordinary
// file-state usage and must stay silent.

const evoFileNoPatchInScopeSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI059_NoPatchInScope_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("configure.go", evoFileNoPatchInScopeSrc, patchFileSetPin)
	for _, f := range res.Findings {
		if f.RuleID == "API-059" {
			t.Fatalf("false positive API-059 with no evo.Patch call in scope: %+v", f)
		}
	}
}

func TestAPI059_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("apply.go", patchFileSetDiscardedNamedVarSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-059" {
			t.Fatalf("API-059 fired for a pin older than 1.1.0 (evo.Patch did not exist yet): %+v", f)
		}
	}
}

func TestAPI059_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSourceAt("apply.go", patchFileSetDiscardedNamedVarSrc, patchFileSetPin)
	findingByID(t, res, "API-059")

	after := review.GoSourceAt("apply_good.go", patchFilesGoodCodeSrc, patchFileSetPin)
	for _, f := range after.Findings {
		if f.RuleID == "API-059" {
			t.Fatalf("API-059 still fires after remediation to evo.Files: %+v", f)
		}
	}
}

func TestAPI059_CurrentDialectNeverFires(t *testing.T) {
	for _, pin := range []string{"", "2.0.0"} {
		res := review.GoSourceAt("apply.go", patchFileSetDiscardedNamedVarSrc, pin)
		for _, f := range res.Findings {
			if f.RuleID == "API-059" {
				t.Fatalf("API-059 fired for pin %q; evo.Files was removed in 2.0: %+v", pin, f)
			}
		}
	}
}
