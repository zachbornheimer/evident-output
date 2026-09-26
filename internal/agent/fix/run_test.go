package fix_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/fix"
)

// writeModule writes a throwaway module rooted at dir that replaces the
// evo module with this checkout, so the fixtures below type-check (or
// fail to, on purpose) against the real evo package the way a real
// consumer repo would.
func writeModule(t *testing.T, dir, src string) {
	t.Helper()
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot = filepath.Dir(filepath.Dir(filepath.Dir(repoRoot))) // internal/agent/fix -> repo root
	mod := "module fixture\n\ngo 1.25\n\nrequire github.com/zachbornheimer/evident-output v0.0.0\n" +
		"replace github.com/zachbornheimer/evident-output => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
}

const fixtureSrc = `package main

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")
	t.Warn("stale cache")
	t.Step(1, 3, "scanning")
	t.Define(func(ctx context.Context) error {
		t.Kept(evo.Reason("dirty"))
		return nil
	})
	return out.Finish()
}

func main() { fmt.Println(run()) }
`

func TestDiagnoseFindsEveryRemovedName(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, fixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 package result, got %d", len(results))
	}
	got := map[string]bool{}
	for _, d := range results[0].Diagnostics {
		got[d.RuleID] = true
	}
	for _, want := range []string{"API-070", "API-090", "API-091"} {
		if !got[want] {
			t.Errorf("missing diagnostic %s; got %v", want, got)
		}
	}
}

const variadicWarnFixtureSrc = `package main

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")
	opts := []evo.ProblemOption{}
	t.Define(func(ctx context.Context) error {
		t.Warn("stale cache", opts...)
		return nil
	})
	return out.Finish()
}

func main() { fmt.Println(run()) }
`

// TestWarnAnalyzerSkipsVariadicSpread guards the API-070 fix against
// producing NewText that appends a plain arg after a spread trailing
// argument (t.Warn("x", opts...) -> t.Problem("x", opts, evo.Severity(...)...)),
// which does not compile: "too many arguments in call". A spread call
// site gets a diagnostic with no auto-fix instead.
func TestWarnAnalyzerSkipsVariadicSpread(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, variadicWarnFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, true)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 package result, got %d", len(results))
	}
	var found bool
	for _, d := range results[0].Diagnostics {
		if d.RuleID != "API-070" {
			continue
		}
		found = true
		if d.Fixed {
			t.Fatalf("variadic Warn call must not have been auto-fixed: %+v", d)
		}
	}
	if !found {
		t.Fatal("missing API-070 diagnostic for variadic Warn call")
	}
}

const packageWarnFixtureSrc = `package main

import (
	e "github.com/zachbornheimer/evident-output"
)

func run() error {
	e.Warn("run-level")
	return nil
}

func main() { _ = run() }
`

// TestWarnAnalyzerFindsPackageLevelWarn guards API-070 detection of the
// package-level evo.Warn (removed in 1.1) called through a non-default
// import alias. isEvoPackageSelector resolves the alias identifier's own
// PkgName through go/types rather than the removed Warn selector, which
// no longer has a Use — a prior version of this analyzer relied on
// packageFunc, which needs a Use on the Warn identifier and so never
// fires once Warn no longer exists in the evo package.
func TestWarnAnalyzerFindsPackageLevelWarn(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, packageWarnFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 package result, got %d", len(results))
	}
	for _, d := range results[0].Diagnostics {
		if d.RuleID == "API-070" {
			return
		}
	}
	t.Fatal("missing API-070 diagnostic for package-level evo.Warn call")
}

const optionsFixtureSrc = `package main

import evo "github.com/zachbornheimer/evident-output"

func run() error {
	out := evo.Init(evo.Title("demo"), evo.DryRun(), evo.Width(80))
	_ = out
	return nil
}
`

func TestOptionsAnalyzerRewritesToConfigLiteral(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, optionsFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, true)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 || len(results[0].Diagnostics) != 1 {
		t.Fatalf("want exactly 1 diagnostic, got %+v", results)
	}
	if !results[0].Diagnostics[0].Fixed {
		t.Fatalf("expected the options rewrite to be fixed, got %+v", results[0].Diagnostics[0])
	}
	out, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "evo.Config{") {
		t.Errorf("expected a rewritten evo.Config{...} literal, got:\n%s", out)
	}
	if strings.Contains(string(out), "evo.Title(") {
		t.Errorf("expected the functional-option call to be gone, got:\n%s", out)
	}
}

const unmappableOptionFixtureSrc = `package main

import (
	"bytes"

	evo "github.com/zachbornheimer/evident-output"
)

func run() error {
	var buf bytes.Buffer
	out := evo.Init(evo.Title("demo"), evo.AlsoWrite(&buf))
	_ = out
	return nil
}
`

func TestOptionsAnalyzerSkipsUnmappableConstructor(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, unmappableOptionFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, true)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 || len(results[0].Diagnostics) != 1 {
		t.Fatalf("want exactly 1 diagnostic, got %+v", results)
	}
	if results[0].Diagnostics[0].Fixed {
		t.Fatalf("AlsoWrite has no Config field: expected no fix, got %+v", results[0].Diagnostics[0])
	}
	if !strings.Contains(results[0].Diagnostics[0].Message, "AlsoWrite") {
		t.Errorf("expected the diagnostic to name the unmapped constructor, got %q", results[0].Diagnostics[0].Message)
	}
}

// aliasedOptionsFixtureSrc imports evo under a non-default alias and uses
// a Debug sub-option plus VisibilityDelay, both of which used to hardcode
// a bare "evo." prefix in the generated Config{} literal regardless of the
// import alias in scope — producing an undefined "evo" reference that
// failed to compile after -apply.
const aliasedOptionsFixtureSrc = `package main

import e "github.com/zachbornheimer/evident-output"

func run() error {
	out := e.Init(e.NoColor(), e.DebugAddSource(), e.Title("t"))
	_ = out
	return nil
}

func main() { _ = run() }
`

func TestOptionsAnalyzerRewritesUnderImportAlias(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, aliasedOptionsFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, true)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 || len(results[0].Diagnostics) != 1 {
		t.Fatalf("want exactly 1 diagnostic, got %+v", results)
	}
	if !results[0].Diagnostics[0].Fixed {
		t.Fatalf("expected the aliased options rewrite to be fixed, got %+v", results[0].Diagnostics[0])
	}
	out, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(out)
	if strings.Contains(src, "\"evo\"") || strings.Contains(src, " evo.") {
		t.Errorf("expected every generated reference to use the source's own alias \"e\", got:\n%s", src)
	}
	if !strings.Contains(src, "e.Config{") || !strings.Contains(src, "e.ColorNever") || !strings.Contains(src, "e.DebugConfig{") {
		t.Errorf("expected e.Config{...} with e.ColorNever/e.DebugConfig{...}, got:\n%s", src)
	}
	cmd := exec.Command("go", "build", "-buildvcs=false", "./...")
	cmd.Dir = dir
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build after -apply: %v\n%s", err, buildOut)
	}
}

// lookalikeFixtureSrc defines a local type whose methods spell the exact
// names the 1.1 vocabulary freeze retired or renamed on evo's own types —
// Warn, Step, Kept, Blockf — but which has nothing to do with evo. It also
// calls the real evo.Task/Reason to prove those still get flagged in the
// same file. Detection in this package must resolve every call through
// pass.TypesInfo back to a declared evo type or function, never by
// matching an identifier's spelling: a false positive here would rewrite
// an unrelated type's call into text that does not compile.
const lookalikeFixtureSrc = `package main

import evo "github.com/zachbornheimer/evident-output"

type logger struct{}

func (l *logger) Warn(msg string)                   {}
func (l *logger) Step(completed, total int, s string) {}
func (l *logger) Kept(reason string)                 {}
func (l *logger) Blockf(format string, args ...any)  {}

func run() error {
	l := &logger{}
	l.Warn("stale cache")
	l.Step(1, 3, "scanning")
	l.Kept("dirty")
	l.Blockf("boom %d", 1)

	out := evo.Init(evo.Config{Title: "demo"})
	out.Task("check").Kept(evo.Reason("dirty"))
	return nil
}

func main() { _ = run() }
`

// TestLookalikeMethodsOnNonEvoTypeAreNeverFlagged is the negative test for
// the typed-only detection contract: a non-evo type with methods named
// Warn/Step/Kept/Blockf must never be flagged by any analyzer in this
// package, even though its method names alias every retired evo name
// (plus Blockf, which is not retired at all). The real evo.TaskHandle.Kept
// call in the same fixture is the positive control confirming the
// analyzers still ran.
func TestLookalikeMethodsOnNonEvoTypeAreNeverFlagged(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, lookalikeFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 package result, got %d", len(results))
	}
	var sawRealKept bool
	for _, d := range results[0].Diagnostics {
		if d.Line == 0 {
			t.Fatalf("unexpected diagnostic with no source line: %+v", d)
		}
		if d.RuleID == "API-091" {
			sawRealKept = true
			continue
		}
		t.Errorf("lookalike method on non-evo type was flagged: %+v", d)
	}
	if !sawRealKept {
		t.Fatal("expected the real evo.TaskHandle.Kept call to still be flagged (API-091); analyzers may not have run")
	}
}

func TestDiagnoseApplyConvergesToNoDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, fixtureSrc)

	for i := range 4 {
		pkgs, err := fix.Load(dir, ".")
		if err != nil {
			t.Fatalf("Load round %d: %v", i, err)
		}
		results, err := fix.Diagnose(pkgs, true)
		if err != nil {
			t.Fatalf("Diagnose round %d: %v", i, err)
		}
		unfixed := 0
		for _, r := range results {
			for _, d := range r.Diagnostics {
				if !d.Fixed {
					unfixed++
				}
			}
		}
		if unfixed == 0 {
			out, err := os.ReadFile(filepath.Join(dir, "main.go"))
			if err != nil {
				t.Fatal(err)
			}
			for _, retired := range []string{".Warn(", ".Step(", ".Kept("} {
				if strings.Contains(string(out), retired) {
					t.Errorf("converged output still contains %s:\n%s", retired, out)
				}
			}
			return
		}
	}
	t.Fatal("fix -apply did not converge to zero unfixed diagnostics within 4 rounds")
}

// methodValueFixtureSrc exercises the method-value (f := t.Warn, s :=
// t.Step) and method-expression ((*evo.TaskHandle).Step,
// (*evo.TaskHandle).Kept) shapes: a stand-alone reference to a removed
// name with no CallExpr wrapping it at the reference site, which a
// call-based Preorder walk never sees. defer t.Step(...) below is a
// plain call (defer always wraps a call), not a method value — it
// exercises the ordinary call-based path alongside the value/expression
// shapes.
const methodValueFixtureSrc = `package main

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	warn := t.Warn
	warn("stale cache")

	step := t.Step
	defer step(1, 3, "cleanup")

	stepExpr := (*evo.TaskHandle).Step

	t.Define(func(ctx context.Context) error {
		stepExpr(t, 2, 3, "define")
		keptFn := (*evo.TaskHandle).Kept
		keptFn(t, evo.Reason("dirty"))
		return nil
	})
	return out.Finish()
}

func main() { fmt.Println(run()) }
`

// TestMethodValuesAndExpressionsAreFlagged guards the method-value/method-
// expression detection: f := t.Warn, s := t.Step (both method values),
// and the method expressions (*evo.TaskHandle).Step and
// (*evo.TaskHandle).Kept must all be flagged, each with a SuggestedFix
// that -apply can converge on.
func TestMethodValuesAndExpressionsAreFlagged(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, methodValueFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 package result, got %d", len(results))
	}
	got := map[string]bool{}
	for _, d := range results[0].Diagnostics {
		got[d.RuleID] = true
	}
	for _, want := range []string{"API-070", "API-090", "API-091"} {
		if !got[want] {
			t.Errorf("missing diagnostic %s for a method value/expression reference; got %v", want, got)
		}
	}
}

// TestMethodValueFixApplyCompiles guards that the generated func-literal
// wrapper for a method value actually compiles once applied — not just
// that a SuggestedFix was offered.
func TestMethodValueFixApplyCompiles(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, methodValueFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := fix.Diagnose(pkgs, true); err != nil {
		t.Fatalf("Diagnose -apply: %v", err)
	}

	cmd := exec.Command("go", "build", "-buildvcs=false", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build after -apply: %v\n%s", err, out)
	}
}

// manualWarnValueFixtureSrc exercises the two API-070 method-value/
// method-expression receivers that have no Task to attach a Problem to:
// evo.Output.Warn used as a func value, and the package-level evo.Warn
// used as a func value. Neither is mechanical, so both must be flagged
// with a diagnostic naming the manual step and no SuggestedFix that
// rewrites the reference site.
const manualWarnValueFixtureSrc = `package main

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})

	outputWarn := out.Warn
	outputWarn("stale cache")

	pkgWarn := evo.Warn
	pkgWarn("no task yet")

	return out.Finish()
}

func main() { fmt.Println(run()) }
`

// TestManualWarnValueBranchesAreFlaggedWithoutFix guards the two
// unfixable API-070 method-value receivers: Output.Warn and the
// package-level evo.Warn, both used as func values. Detection must fire
// for both, and -apply must leave the reference sites untouched (no
// SuggestedFix exists to apply), unlike the TaskHandle case in
// TestMethodValueFixApplyCompiles.
func TestManualWarnValueBranchesAreFlaggedWithoutFix(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, manualWarnValueFixtureSrc)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 package result, got %d", len(results))
	}
	count := 0
	for _, d := range results[0].Diagnostics {
		if d.RuleID == "API-070" {
			count++
			if !strings.Contains(d.Message, "by hand") {
				t.Errorf("expected a manual-step message for an unfixable Warn value, got %q", d.Message)
			}
		}
	}
	if count != 2 {
		t.Fatalf("want 2 API-070 diagnostics (Output.Warn and package-level evo.Warn as values), got %d", count)
	}

	// -apply must not rewrite either reference site: there is no
	// mechanical fix for a receiver with no Task to attach a Problem to.
	pkgs, err = fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := fix.Diagnose(pkgs, true); err != nil {
		t.Fatalf("Diagnose -apply: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "outputWarn := out.Warn") || !strings.Contains(string(out), "pkgWarn := evo.Warn") {
		t.Errorf("expected the unfixable Warn value references to remain untouched after -apply:\n%s", out)
	}
}
