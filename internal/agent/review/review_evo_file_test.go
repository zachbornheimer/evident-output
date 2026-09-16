package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// EVO-FILE-001: manual os.WriteFile + os.Chmod on the same path is exactly
// the boilerplate evo.File's Path/Contents/Mode/Basis fields replace
// (spec §62 "manual common file reconciliation → simplification finding").
func TestEVOFILE001_ManualWriteAndChmod_EmitsSimplificationFinding(t *testing.T) {
	src := `package p
import (
	"fmt"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func writeConfig(out *evo.Output, path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}
`
	res := review.GoSource("launchd.go", src)
	f := findingByID(t, res, "EVO-FILE-001")
	if f.Severity != "suggestion" {
		t.Fatalf("severity = %q, want suggestion", f.Severity)
	}
	if f.Suggestion == "" {
		t.Fatal("EVO-FILE-001 suggestion is empty")
	}
}

// evo.File used declaratively must never itself be flagged — it performs no
// direct os.WriteFile/os.Chmod call site (spec §62 "evo.File desired state → no finding").
func TestEVOFILE001_DeclarativeEvoFile_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func writeConfig(ctx context.Context, path string, data []byte, mode int, basis []evo.Fingerprint) error {
	return evo.File(ctx, evo.FileSpec{Path: path, Contents: data, Mode: mode, Basis: basis})
}
`
	res := review.GoSource("config.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "EVO-FILE-001" {
			t.Fatalf("false positive EVO-FILE-001 on declarative evo.File: %+v", f)
		}
	}
}

// EVO-FILE-001 (freshness-boundary warning): generate(ctx) already ran
// before the trailing evo.File return — tracking cannot retroactively skip
// work that already executed (spec §7, §62 "expensive work before trailing
// evo.File → warning that tracking cannot retroactively skip it").
func TestEVOFILE001_ExpensiveWorkBeforeTrailingFile_EmitsWarning(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func writeReport(ctx context.Context, output string, basis []evo.Fingerprint) error {
	data, err := generate(ctx)
	if err != nil {
		return err
	}
	return evo.File(ctx, evo.FileSpec{Path: output, Contents: data, Mode: 0o644, Basis: basis})
}
func generate(ctx context.Context) ([]byte, error) { return nil, nil }
`
	res := review.GoSource("report.go", src)
	f := findingByID(t, res, "EVO-FILE-001")
	if f.Severity != "warning" {
		t.Fatalf("severity = %q, want warning", f.Severity)
	}
}

// EVO-EXEC-001: raw exec.Command guarded by a hand-rolled staleness check
// duplicates evo.Exec's Basis/Outputs freshness contract (spec §62 "raw
// exec + manual output hashes → evo.Exec suggestion").
func TestEVOEXEC001_RawExecWithManualFreshness_EmitsSuggestion(t *testing.T) {
	src := `package p
import (
	"os/exec"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(out *evo.Output, inputPath, outPath string) error {
	if outputIsStale(inputPath, outPath) {
		cmd := exec.Command("python3", "generate.py", inputPath, outPath)
		return cmd.Run()
	}
	return nil
}
func outputIsStale(a, b string) bool { return true }
`
	res := review.GoSource("generate.go", src)
	f := findingByID(t, res, "EVO-EXEC-001")
	if f.Suggestion == "" {
		t.Fatal("EVO-EXEC-001 suggestion is empty")
	}
}

// evo.Exec with a declared Basis/Outputs contract must never itself be
// flagged (spec §62 "evo.Exec generated output + Basis → no finding").
func TestEVOEXEC001_DeclarativeEvoExec_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(ctx context.Context, inputPath, outPath string) error {
	return evo.Exec(ctx, evo.ExecSpec{
		Executable: "python3",
		Args:       []string{"generate.py", inputPath, outPath},
		Basis:      []evo.Fingerprint{evo.FSPath(inputPath)},
		Outputs:    []string{outPath},
	})
}
`
	res := review.GoSource("generate.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "EVO-EXEC-001" {
			t.Fatalf("false positive EVO-EXEC-001 on declarative evo.Exec: %+v", f)
		}
	}
}

// EVO-PROVENANCE-001/002 have no static detector (Detection: "guidance" —
// see rules.Explain); this regression suite instead proves the reviewer
// never fabricates a false claim for either shape (spec §62 "opaque
// callback → no false claim of precise provenance", "same app fingerprint +
// prior manifest → no false whole-Task skip claim").
func TestEVOPROVENANCE_OpaqueCallback_NoFalseProvenanceClaim(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle) {
	task.Verify(func(ctx context.Context) (bool, error) {
		return externalCheck(ctx)
	})
}
func externalCheck(ctx context.Context) (bool, error) { return true, nil }
`
	res := review.GoSource("verify.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "EVO-PROVENANCE-001" || f.RuleID == "EVO-PROVENANCE-002" {
			t.Fatalf("reviewer fabricated a provenance claim for an opaque callback: %+v", f)
		}
	}
}

func TestEVOPROVENANCE_PriorManifestSameFingerprint_NoFalseSkipClaim(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, manifest Manifest) {
	task.Verify(func(ctx context.Context) (bool, error) {
		return manifest.Fingerprint() == evo.App(), nil
	})
}
type Manifest struct{}
func (Manifest) Fingerprint() evo.Fingerprint { return nil }
`
	res := review.GoSource("verify.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "EVO-PROVENANCE-001" || f.RuleID == "EVO-PROVENANCE-002" {
			t.Fatalf("reviewer fabricated a whole-Task skip claim from a manifest fingerprint match: %+v", f)
		}
	}
}
