package review_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/sections"
)

// EVO-FILE-001: manual os.WriteFile + os.Chmod on the same path is exactly
// the boilerplate evo.File's Path/Contents/Mode/Inputs fields replace
// (spec §62 "manual common file reconciliation → simplification finding").
func TestEVOFILE001_ManualWriteAndChmod_EmitsSimplificationFinding(t *testing.T) {
	src := `package p
import (
	"fmt"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func writeConfig(r *evo.Run, path string, data []byte) error {
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
	"io/fs"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func writeConfig(ctx context.Context, path string, data []byte, mode fs.FileMode, inputs []string) error {
	return evo.File(ctx, evo.FileSpec{Path: path, Contents: data, Mode: mode, Inputs: inputs})
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
	evo "github.com/zachbornheimer/evident-output/v2"
)
func writeReport(ctx context.Context, output string, inputs []string) error {
	data, err := generate(ctx)
	if err != nil {
		return err
	}
	return evo.File(ctx, evo.FileSpec{Path: output, Contents: data, Mode: 0o644, Inputs: inputs})
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
	evo "github.com/zachbornheimer/evident-output/v2"
)
func writeConfig(ctx context.Context, path string, data []byte, inputs []string) error {
	key := sanitizeKey(path)
	return evo.File(ctx, evo.FileSpec{Path: key, Contents: data, Mode: 0o644, Inputs: inputs})
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
// duplicates evo.Exec's Inputs/Outputs freshness contract (spec §62 "raw
// exec + manual output hashes → evo.Exec suggestion").
func TestEVOEXEC001_RawExecWithManualFreshness_EmitsSuggestion(t *testing.T) {
	src := `package p
import (
	"os/exec"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(r *evo.Run, inputPath, outPath string) error {
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

// evo.Exec with a declared Inputs/Outputs contract must never itself be
// flagged (spec §62 "evo.Exec generated output + Inputs → no finding").
func TestEVOEXEC001_DeclarativeEvoExec_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context, inputPath, outPath string) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:    []string{"python3", "generate.py", inputPath, outPath},
		Inputs:  []string{"generate.py", inputPath},
		Outputs: []string{outPath},
	})
	return err
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
// same function that builds an evo.File call is flagged when that call's
// Inputs omit it (spec §57's own "template.txt is read but omitted").
func TestEVOPROVENANCE001_VisibleReadOmittedFromInputs_EmitsFindingAtReadLine(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context, out string) error {
	data, _ := os.ReadFile("template.txt")
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Inputs:   []string{"input.xlsx"},
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
	if !strings.Contains(found[0].Suggestion, `"template.txt" to FileSpec.Inputs`) {
		t.Fatalf("suggestion must name the omitted path and FileSpec.Inputs: %q", found[0].Suggestion)
	}
}

// The same read, with the path also listed in Inputs, must never be flagged.
func TestEVOPROVENANCE001_VisibleReadListedInInputs_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context, out string) error {
	data, _ := os.ReadFile("template.txt")
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Inputs:   []string{"input.xlsx", "template.txt"},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 with path present in Inputs: %+v", found)
	}
}

// template.ParseFiles reads every path it is given.
func TestEVOPROVENANCE001_TemplateParseFilesOmittedFromInputs_EmitsFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"text/template"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context, out string) error {
	tmpl, _ := template.ParseFiles("page.tmpl", "layout.tmpl")
	_ = tmpl
	return evo.File(ctx, evo.FileSpec{Path: out, Inputs: []string{"layout.tmpl"}})
}
`
	found := findingsByID(review.GoSource("generate.go", src), "EVO-PROVENANCE-001")
	if len(found) != 1 || found[0].Line != 8 {
		t.Fatalf("want exactly one finding at line 8 (page.tmpl), got %+v", found)
	}
}

// A read outside any function that also builds an evo.File/evo.Exec call has
// no Inputs to compare against, so it is never flagged — the rule is about a
// callback's own claimed freshness, not every os.ReadFile in the package.
func TestEVOPROVENANCE001_ReadOutsideAnyFileCallback_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func loadTemplate() ([]byte, error) {
	return os.ReadFile("template.txt")
}
func generate(ctx context.Context, out string, data []byte) error {
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Inputs:   []string{"input.xlsx"},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 from an unrelated function's read: %+v", found)
	}
}

// A read in one Define callback is not tied to the File or Exec call of a
// sibling callback in the same enclosing function: each function literal is
// its own body.
func TestEVOPROVENANCE001_ReadInSiblingCallback_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
	r.Task("load").Define(func(context.Context) error {
		_, err := os.ReadFile("template.txt")
		return err
	})
	r.Task("write").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "out.txt", Inputs: []string{"input.xlsx"}})
	})
	return nil
}
`
	if found := findingsByID(review.GoSource("run.go", src), "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 across sibling callbacks: %+v", found)
	}
}

// A variable path is never inferred — only a string literal the source
// itself spells out is ever compared against Inputs.
func TestEVOPROVENANCE001_VariablePathRead_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context, out, templatePath string) error {
	data, _ := os.ReadFile(templatePath)
	return evo.File(ctx, evo.FileSpec{
		Path:     out,
		Contents: data,
		Mode:     0o644,
		Inputs:   []string{"input.xlsx"},
	})
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 on a variable path: %+v", found)
	}
}

// EVO-PROVENANCE-001's other shape: a literal Exec Argv entry naming a file,
// not listed in that same call's Inputs.
func TestEVOPROVENANCE001_LiteralExecArgvOmittedFromInputs_EmitsFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:    []string{"python3", "--input", "template.txt"},
		Inputs:  []string{},
		Outputs: []string{"report.json"},
	})
	return err
}
`
	res := review.GoSource("generate.go", src)
	found := findingsByID(res, "EVO-PROVENANCE-001")
	if len(found) != 1 {
		t.Fatalf("got %d EVO-PROVENANCE-001 findings, want exactly 1: %+v", len(found), found)
	}
	if want := 8; found[0].Line != want {
		t.Fatalf("Line = %d, want %d (the \"template.txt\" Argv literal)", found[0].Line, want)
	}
}

// The same Argv entry, listed in Inputs, must never be flagged.
func TestEVOPROVENANCE001_LiteralExecArgvListedInInputs_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:   []string{"python3", "--input", "template.txt"},
		Inputs: []string{"template.txt"},
	})
	return err
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 with Argv path present in Inputs: %+v", found)
	}
}

// An Exec with no Outputs, Inputs or Values always runs, so it claims no
// freshness for an unlisted file to falsify.
func TestEVOPROVENANCE001_ExecWithoutFreshnessClaim_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"python3", "template.txt"}})
	return err
}
`
	if found := findingsByID(review.GoSource("generate.go", src), "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 on an always-run Exec: %+v", found)
	}
}

// An Argv entry naming a path this same call declares as an Output is not an
// undeclared input — it is where the child writes, not something the
// callback reads and must fingerprint. Never flagged.
func TestEVOPROVENANCE001_LiteralExecArgvListedInOutputs_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:    []string{"python3", "--out", "report.json"},
		Inputs:  []string{},
		Outputs: []string{"report.json"},
	})
	return err
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 with Argv path present in Outputs: %+v", found)
	}
}

// Argv[0] names the program, never a file the callback reads: a subcommand
// shape that repeats the tool's own name is not an undeclared input.
func TestEVOPROVENANCE001_ExecProgramRepeatedInArgv_NoFinding(t *testing.T) {
	src := `package p
import (
	"context"
	evo "github.com/zachbornheimer/evident-output/v2"
)
func generate(ctx context.Context) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:   []string{"tool.sh", "--self", "tool.sh"},
		Inputs: []string{},
	})
	return err
}
`
	res := review.GoSource("generate.go", src)
	if found := findingsByID(res, "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("false positive EVO-PROVENANCE-001 with Argv entry equal to the program: %+v", found)
	}
}

// TestProvenanceRuleDetectsUndeclaredInput reviews the program the gate-3
// review wrote against CONTRACT.md: its generate Task runs a script whose
// template.txt (and script) it never lists in Inputs.
func TestProvenanceRuleDetectsUndeclaredInput(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "provenance", "undeclared_input", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	found := findingsByID(review.GoSource("main.go", string(src)), "EVO-PROVENANCE-001")
	paths := map[string]bool{}
	for _, f := range found {
		for _, path := range []string{"template.txt", "generate.py"} {
			if strings.Contains(f.Message, path) {
				paths[path] = true
			}
		}
		if !strings.Contains(f.Suggestion, "ExecSpec.Inputs") {
			t.Errorf("suggestion must name ExecSpec.Inputs, got %q", f.Suggestion)
		}
	}
	if len(found) != 2 || !paths["template.txt"] || !paths["generate.py"] {
		t.Fatalf("want one finding each for template.txt and generate.py, got %+v", found)
	}
}

// TestProvenanceIgnoresShellScriptOperands reviews the gate-4 probe whose
// concat Task runs `sh -c "cat a.txt b.txt c.txt > all.txt"` with every file
// already declared in Inputs/Outputs: the script operand is one string, not a
// path, and must never be reported as an omitted input. A real file Argv entry
// beside it still is.
func TestProvenanceIgnoresShellScriptOperands(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "provenance", "shell_script_operands", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if found := findingsByID(review.GoSource("main.go", string(src)), "EVO-PROVENANCE-001"); len(found) != 0 {
		t.Fatalf("shell script operand reported as an omitted input: %+v", found)
	}
	for name, argv := range map[string]string{
		"sh -c whitespace-free script":  `"sh", "-c", "build.sh"`,
		"python -c script":              `"python3", "-c", "import x; x.go()", "real.txt"`,
		"perl -e script":                `"perl", "-e", "print 1 > out.log", "real.txt"`,
		"metacharacter-bearing literal": `"tool", "a.txt>b.txt", "real.txt"`,
	} {
		src := "package p\nimport (\n\t\"context\"\n\tevo \"github.com/zachbornheimer/evident-output/v2\"\n)\n" +
			"func f(ctx context.Context) error {\n\t_, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{" + argv + "}, Outputs: []string{\"o\"}})\n\treturn err\n}\n"
		found := findingsByID(review.GoSource("p.go", src), "EVO-PROVENANCE-001")
		wantReal := name != "sh -c whitespace-free script"
		if wantReal != (len(found) == 1 && strings.Contains(found[0].Message, "real.txt")) || len(found) > 1 {
			t.Errorf("%s: want a finding for real.txt only=%v, got %+v", name, wantReal, found)
		}
	}
}

// The rule's own BadCode raises the finding and its GoodCode raises none and
// never wraps a nil error.
func TestProvenanceRuleExamplesRoundTrip(t *testing.T) {
	r, ok := rules.Explain("EVO-PROVENANCE-001")
	if !ok {
		t.Fatal("EVO-PROVENANCE-001 is not registered in rules.Explain")
	}
	wrap := func(body string) string {
		return "package p\nimport (\n\t\"context\"\n\t\"fmt\"\n\tevo \"github.com/zachbornheimer/evident-output/v2\"\n)\n" +
			"var _ = fmt.Errorf\nfunc generate(ctx context.Context) error {\n" + body + "\n}\n"
	}
	if got := findingsByID(review.GoSource("bad.go", wrap(r.BadCode)), "EVO-PROVENANCE-001"); len(got) != 1 {
		t.Errorf("BadCode must raise exactly one EVO-PROVENANCE-001 finding, got %+v", got)
	}
	if got := findingsByID(review.GoSource("good.go", wrap(r.GoodCode)), "EVO-PROVENANCE-001"); len(got) != 0 {
		t.Errorf("GoodCode raises EVO-PROVENANCE-001: %+v", got)
	}
	if !strings.Contains(r.GoodCode, "err != nil") {
		t.Errorf("GoodCode must wrap the error only when there is one:\n%s", r.GoodCode)
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
