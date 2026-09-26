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

// removedNameFixtureSrc is the same removed-name call site used across
// this file's tests: one Warn, one Step, one Kept(ForSkip).
const removedNameFixtureSrc = `package sub

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

// TestGoDirectory_RemovedNameAnalyzers_SubdirectoryOfModule pins the
// review-gap report's REGRESSION case: reviewing a package subdirectory
// (not the module root itself) must still resolve the module's own go.mod
// by walking up from dir, not just os.Stat(dir/go.mod).
func TestGoDirectory_RemovedNameAnalyzers_SubdirectoryOfModule(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "internal", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// writeRemovedNameModule's own main.go (root) also imports evo, keeping
	// the module tidy; the removed-name fixture under review lives only in
	// the subdirectory being reviewed.
	writeRemovedNameModule(t, root, removedNameFixtureSrc)
	if err := os.WriteFile(filepath.Join(sub, "app.go"), []byte(removedNameFixtureSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := review.GoDirectory(sub)
	if err != nil {
		t.Fatalf("GoDirectory: %v", err)
	}
	got := map[string]bool{}
	for _, f := range res.Findings {
		got[f.RuleID] = true
	}
	for _, want := range []string{"API-070", "API-090", "API-091", "API-120"} {
		if !got[want] {
			t.Errorf("missing %s reviewing a subdirectory of the module root; findings=%+v", want, res.Findings)
		}
	}
	if res.Partial {
		t.Errorf("a clean, tidied module must not report Partial: %+v", res)
	}
}

// TestGoDirectory_RemovedNameAnalyzers_PartialOnUntidiedModule pins the
// review-gap report's second finding: a go.mod that exists but whose
// module graph can't resolve (no `go mod tidy` run yet, mid-migration)
// must surface as Result.Partial=true, not silent zero findings.
func TestGoDirectory_RemovedNameAnalyzers_PartialOnUntidiedModule(t *testing.T) {
	dir := t.TempDir()
	mod := "module fixture\n\ngo 1.25\n\nrequire github.com/zachbornheimer/evident-output v0.0.0\n" +
		"replace github.com/zachbornheimer/evident-output => /does/not/exist\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nimport evo \"github.com/zachbornheimer/evident-output\"\n\nfunc main() { evo.Init(evo.Config{}) }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// deliberately no `go mod tidy`: go.sum is missing / the replace target
	// doesn't exist, so packages.Load cannot resolve the module graph.

	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatalf("GoDirectory: %v", err)
	}
	if !res.Partial {
		t.Errorf("an untidied module with an unresolvable replace must report Partial=true, got %+v", res)
	}
}

// TestGoFileAt_ReportsRemovedNames pins the review-gap report's other
// REGRESSION case: reviewing a single file (the CLI's `review file.go` path
// and the MCP kind=go default with an absolute `file`) must still report
// API-070/090/091/120, by resolving the file's own module.
func TestGoFileAt_ReportsRemovedNames(t *testing.T) {
	dir := t.TempDir()
	writeRemovedNameModule(t, dir, removedNameFixtureSrc)

	res, err := review.GoFileAt(filepath.Join(dir, "main.go"), "")
	if err != nil {
		t.Fatalf("GoFileAt: %v", err)
	}
	got := map[string]bool{}
	for _, f := range res.Findings {
		got[f.RuleID] = true
	}
	for _, want := range []string{"API-070", "API-090", "API-091", "API-120"} {
		if !got[want] {
			t.Errorf("missing %s from single-file review; findings=%+v", want, res.Findings)
		}
	}
}

// TestNoFileDetectorEmitsRemovedNameRuleIDs guards directory.go's dropped
// de-duplication step: it was removed because no per-file GoSourceAt
// detector emits API-070/090/091/120 any more (they're analyzer-only, see
// detectors.go). If a future detector starts emitting one of these IDs
// again, a directory review would double-report it — this test is the
// tripwire for that regression, not GoDirectoryAt's own de-dup.
func TestNoFileDetectorEmitsRemovedNameRuleIDs(t *testing.T) {
	res := review.GoSource("main.go", removedNameFixtureSrc)
	for _, f := range res.Findings {
		switch f.RuleID {
		case "API-070", "API-090", "API-091", "API-120":
			t.Errorf("a per-file detector emitted %s; directory.go's removed-name merge assumes this never happens: %+v", f.RuleID, f)
		}
	}
}
