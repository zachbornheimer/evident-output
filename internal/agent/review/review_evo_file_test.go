package review_test

import (
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/internal/agent/sections"
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

// EVO-FILE-001 (freshness-boundary warning) must not fire on a cheap,
// non-context-taking local helper before the trailing evo.File return —
// only work that could plausibly be expensive/mutating (a call threaded
// with ctx) is in scope for the warning (spec §7).
func TestEVOFILE001_CheapHelperBeforeTrailingFile_NoWarning(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func writeConfig(ctx context.Context, path string, data []byte, basis []evo.Fingerprint) error {
	key := sanitizeKey(path)
	return evo.File(ctx, evo.FileSpec{Path: key, Contents: data, Mode: 0o644, Basis: basis})
}
func sanitizeKey(path string) string { return path }
`
	res := review.GoSource("config.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "EVO-FILE-001" && f.Severity == "warning" {
			t.Fatalf("false positive EVO-FILE-001 warning on cheap helper: %+v", f)
		}
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

// findingsByID returns every finding in res matching id, so a test can
// assert an exact count rather than merely "at least one" or "none".
func findingsByID(res review.Result, id string) []review.Finding {
	var out []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == id {
			out = append(out, f)
		}
	}
	return out
}

// EVO-PROVENANCE-001: a string-literal path read via os.ReadFile in the
// same function that builds an evo.File call's Basis is flagged when that
// Basis omits it (spec §57's own "template.txt is read but omitted").
func TestEVOPROVENANCE001_VisibleReadOmittedFromBasis_EmitsFindingAtReadLine(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(ctx context.Context, out string) error {
	data, _ := os.ReadFile("template.txt")
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Basis:    []evo.Fingerprint{evo.FSPath("input.xlsx")},
	})
}
`
	res := review.GoSource("generate.go", src)
	found := findingsByID(res, "EVO-PROVENANCE-001")
	if len(found) != 1 {
		t.Fatalf("got %d EVO-PROVENANCE-001 findings, want exactly 1: %+v", len(found), found)
	}
	if want := 8; found[0].Line != want {
		t.Fatalf("Line = %d, want %d (the os.ReadFile call)", found[0].Line, want)
	}
}

// The same read, with the path also listed in Basis, must never be flagged.
func TestEVOPROVENANCE001_VisibleReadListedInBasis_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(ctx context.Context, out string) error {
	data, _ := os.ReadFile("template.txt")
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Basis: []evo.Fingerprint{
			evo.FSPath("input.xlsx"),
			evo.FSPath("template.txt"),
		},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 with path present in Basis: %+v", found)
	}
}

// A read that happens outside any function that also builds a Basis-bearing
// evo.File/evo.Exec call has no Basis to compare against, so it is never
// flagged — the rule is about a callback's own claimed freshness, not every
// os.ReadFile in the package.
func TestEVOPROVENANCE001_ReadOutsideAnyBasisCallback_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func loadTemplate() ([]byte, error) {
	return os.ReadFile("template.txt")
}
func generate(ctx context.Context, out string, data []byte) error {
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Basis:    []evo.Fingerprint{evo.FSPath("input.xlsx")},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 from an unrelated function's read: %+v", found)
	}
}

// A variable path is never inferred — only a string literal the source
// itself spells out is ever compared against Basis.
func TestEVOPROVENANCE001_VariablePathRead_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(ctx context.Context, out, templatePath string) error {
	data, _ := os.ReadFile(templatePath)
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Basis:    []evo.Fingerprint{evo.FSPath("input.xlsx")},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 on a variable path: %+v", found)
	}
}

// EVO-PROVENANCE-001's other shape: a literal Exec Arg naming a file, not
// listed via evo.FSPath in that same call's Basis.
func TestEVOPROVENANCE001_LiteralExecArgOmittedFromBasis_EmitsFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(ctx context.Context) error {
	return evo.Exec(ctx, evo.ExecSpec{
		Executable: "python3",
		Args:       []string{"--input", "template.txt"},
		Basis:      []evo.Fingerprint{},
	})
}
`
	res := review.GoSource("generate.go", src)
	found := findingsByID(res, "EVO-PROVENANCE-001")
	if len(found) != 1 {
		t.Fatalf("got %d EVO-PROVENANCE-001 findings, want exactly 1: %+v", len(found), found)
	}
	if want := 9; found[0].Line != want {
		t.Fatalf("Line = %d, want %d (the \"template.txt\" Arg literal)", found[0].Line, want)
	}
}

// The same Exec Arg, listed in Basis, must never be flagged.
func TestEVOPROVENANCE001_LiteralExecArgListedInBasis_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output"
)
func generate(ctx context.Context) error {
	return evo.Exec(ctx, evo.ExecSpec{
		Executable: "python3",
		Args:       []string{"--input", "template.txt"},
		Basis:      []evo.Fingerprint{evo.FSPath("template.txt")},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 with Exec Arg path present in Basis: %+v", found)
	}
}

// EVO-PROVENANCE-002 has no static detector — distinguishing "trusts a
// prior manifest alone" from a legitimate cached-but-reverified check needs
// call-site intent no AST shape carries (rules_provenance.go). This proves
// it stays registered as a fully-documented guidance rule and is surfaced
// through the MCP catalog's provenance guide, rather than silently
// disappearing from both the rule registry and the docs a reviewing agent
// reads.
func TestEVOPROVENANCE002_RegisteredAsGuidanceAndSurfacedInProvenanceCatalogGuide(t *testing.T) {
	r, ok := rules.Explain("EVO-PROVENANCE-002")
	if !ok {
		t.Fatal("EVO-PROVENANCE-002 is not registered in rules.Explain")
	}
	if r.Detection != "guidance" {
		t.Fatalf(`Detection = %q, want "guidance" — EVO-PROVENANCE-002 has no static detector`, r.Detection)
	}
	if r.Invariant == "" {
		t.Fatal("EVO-PROVENANCE-002 missing invariant")
	}
	if r.Remediation == "" {
		t.Fatal("EVO-PROVENANCE-002 missing remediation (migration)")
	}
	if r.Since == "" {
		t.Fatal("EVO-PROVENANCE-002 missing since (target version)")
	}

	var guide catalog.Guide
	guideFound := false
	for _, g := range catalog.All() {
		if g.ID == "provenance" {
			guide, guideFound = g, true
			break
		}
	}
	if !guideFound {
		t.Fatal(`catalog.All() has no "provenance" guide`)
	}
	if !slices.Contains(guide.Rules, "EVO-PROVENANCE-002") {
		t.Fatalf("provenance guide.Rules = %v, want EVO-PROVENANCE-002 listed", guide.Rules)
	}

	sectionFound := false
	for _, s := range sections.List() {
		if s.ID == "guide/provenance" {
			sectionFound = true
			break
		}
	}
	if !sectionFound {
		t.Fatal(`sections.List() does not surface "guide/provenance" — MCP callers cannot read it`)
	}
}
