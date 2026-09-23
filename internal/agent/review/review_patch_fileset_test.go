package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-058 (ZYS-935): a Patch-derived FileSet is never passed to evo.Files —
// instead the same function reconstructs a fresh evo.FileSpec and commits
// through evo.File, discarding the Basis/stale-write guard the FileSet
// carried.

const patchFileSetDiscardedNamedVarSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
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

func TestAPI058_PatchFileSetDiscardedNamedVar_Fires(t *testing.T) {
	res := review.GoSource("apply.go", patchFileSetDiscardedNamedVarSrc)
	f := findingByID(t, res, "API-058")
	if f.Severity != "error" {
		t.Fatalf("API-058 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.Files(ctx, fileSet)") {
		t.Fatalf("API-058 suggestion does not name evo.Files(ctx, fileSet): %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-058 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// The FileSet return value can be discarded outright with "_" instead of
// merely going unused — just as much a strip of the Basis/stale-write
// guard.

const patchFileSetDiscardedBlankSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, diff []byte, path string, contents []byte) error {
  _, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI058_PatchFileSetDiscardedBlank_Fires(t *testing.T) {
	res := review.GoSource("apply_blank.go", patchFileSetDiscardedBlankSrc)
	findingByID(t, res, "API-058")
}

// evo.Files(ctx, fileSet) is the remediated shape and must stay silent.

const patchFilesGoodCodeSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context, diff []byte) error {
  fileSet, err := evo.Patch(ctx, diff)
  if err != nil {
    return err
  }
  return evo.Files(ctx, fileSet)
}
`

func TestAPI058_EvoFilesGoodCode_StaysSilent(t *testing.T) {
	res := review.GoSource("apply_good.go", patchFilesGoodCodeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("false positive API-058 on evo.Files(ctx, fileSet): %+v", f)
		}
	}
}

// A plain evo.File write with no Patch call in scope at all is ordinary
// file-state usage and must stay silent.

const evoFileNoPatchInScopeSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI058_NoPatchInScope_StaysSilent(t *testing.T) {
	res := review.GoSource("configure.go", evoFileNoPatchInScopeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("false positive API-058 with no evo.Patch call in scope: %+v", f)
		}
	}
}

func TestAPI058_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("apply.go", patchFileSetDiscardedNamedVarSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("API-058 fired for a pin older than 1.1.0 (evo.Patch did not exist yet): %+v", f)
		}
	}
}

func TestAPI058_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("apply.go", patchFileSetDiscardedNamedVarSrc)
	findingByID(t, res, "API-058")

	after := review.GoSource("apply_good.go", patchFilesGoodCodeSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-058" {
			t.Fatalf("API-058 still fires after remediation to evo.Files: %+v", f)
		}
	}
}
