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
	t.Kept(evo.Reason("dirty"))
	t.Define(func(ctx context.Context) error {
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
