package review_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// writeRemovedNameModule writes a throwaway module rooted at dir that
// replaces the evo module with this checkout (see fix.writeModule, the
// same pattern), so src type-checks against the real evo package the way
// a real consumer repo would — the precondition
// internal/agent/fix.RemovedNameAnalyzers needs to resolve a call's
// receiver through go/types.
func writeRemovedNameModule(t *testing.T, dir, src string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(wd))) // internal/agent/review -> repo root
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
	if out, tidyErr := cmd.CombinedOutput(); tidyErr != nil {
		t.Fatalf("go mod tidy: %v\n%s", tidyErr, out)
	}
}

// TestGoDirectory_RemovedNameAnalyzers pins the fix-wiring contract this
// slice adds: GoDirectory reports API-070/090/091/120 (Warn/Step/Kept/
// ReasonOption-ForSkip-OnTask, all removed in 1.1) by running
// internal/agent/fix's RemovedNameAnalyzers over the directory's
// type-checked packages, not a second review-owned implementation.
func TestGoDirectory_RemovedNameAnalyzers(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	branches := out.Group("branches")
	t := branches.Task("check")
	t.Warn("tool version differs from manifest")
	t.Step(1, 3, "scanning")
	t.Kept(evo.Reason("dirty", evo.ForSkip()))
	t.Define(func(ctx context.Context) error {
		return nil
	})
	return out.Finish()
}

func main() { fmt.Println(run()) }
`
	writeRemovedNameModule(t, dir, src)

	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatalf("GoDirectory: %v", err)
	}

	got := map[string]bool{}
	for _, f := range res.Findings {
		got[f.RuleID] = true
	}
	for _, want := range []string{"API-070", "API-090", "API-091", "API-120"} {
		if !got[want] {
			t.Errorf("missing %s; findings=%+v", want, res.Findings)
		}
	}
}

// TestGoDirectory_RemovedNameAnalyzers_NoFalsePositiveOnSlogWarn keeps the
// review-gap report's negative case alive under the new wiring: a real
// *slog.Logger's Warn is a different type entirely and must never fire
// API-070 just because the method is spelled the same.
func TestGoDirectory_RemovedNameAnalyzers_NoFalsePositiveOnSlogWarn(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "log/slog"

func run(logger *slog.Logger) {
	logger.Warn("registry request slow", "duration", "4s")
}

func main() { run(nil) }
`
	writeRemovedNameModule(t, dir, src)

	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatalf("GoDirectory: %v", err)
	}
	for _, f := range res.Findings {
		if f.RuleID == "API-070" {
			t.Fatalf("API-070 must not fire on a non-evo receiver's Warn: %+v", f)
		}
	}
}

// TestGoDirectory_RemovedNameAnalyzers_DoesNotFireBelowMinDialect pins the
// AGENTS.md rule directly: a consumer pinned to v1.0.x, where Warn still
// exists and evo.Severity does not, must never be told to rewrite Warn
// into a call that would not compile on that pin.
func TestGoDirectory_RemovedNameAnalyzers_DoesNotFireBelowMinDialect(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import evo "github.com/zachbornheimer/evident-output"

func run(task *evo.TaskHandle) {
	task.Warn("tool version differs from manifest")
}

func main() { run(nil) }
`
	writeRemovedNameModule(t, dir, src)

	res, err := review.GoDirectoryAt(dir, "v1.0.0")
	if err != nil {
		t.Fatalf("GoDirectoryAt: %v", err)
	}
	for _, f := range res.Findings {
		if f.RuleID == "API-070" {
			t.Fatalf("API-070 must not fire for a v1.0.0 pin, where Warn still exists: %+v", f)
		}
	}
}
