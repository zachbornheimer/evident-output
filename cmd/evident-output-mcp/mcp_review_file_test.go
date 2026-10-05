package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// evoViolationSource is real Go source with a genuine evo misuse (STREAM-003:
// fmt.Printf alongside an evo Init) — reading it from disk must reach the
// same review outcome as pasting it inline via `source`.
const evoViolationSource = `package p

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() {
	out := evo.Init(evo.Config{})
	fmt.Printf("progress\n")
	_ = out
}
`

func TestReview_FileParamReadsAbsolutePath(t *testing.T) {
	bin := buildMCP(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "violation.go")
	if err := os.WriteFile(path, []byte(evoViolationSource), 0o600); err != nil {
		t.Fatal(err)
	}
	pathJSON, _ := json.Marshal(path)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":` + string(pathJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("file param did not read disk content, got a parse error instead: %s", out)
	}
	if !strings.Contains(out, "STREAM-003") {
		t.Fatalf("expected STREAM-003 finding from the real file content: %s", out)
	}
}

func TestReview_FileParamMissingPathReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.go")
	pathJSON, _ := json.Marshal(missing)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":` + string(pathJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("expected an honest read-failure error, got a generic parse error: %s", out)
	}
	if !strings.Contains(out, "cannot read") || !strings.Contains(out, missing) {
		t.Fatalf("expected an explicit cannot-read error naming the path, got: %s", out)
	}
}

func TestReview_FilesMapReadsAbsolutePaths(t *testing.T) {
	bin := buildMCP(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "violation.go")
	if err := os.WriteFile(path, []byte(evoViolationSource), 0o600); err != nil {
		t.Fatal(err)
	}
	pathJSON, _ := json.Marshal(path)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"violation.go":` + string(pathJSON) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") || strings.Contains(out, "no files provided") {
		t.Fatalf("files map did not read disk content: %s", out)
	}
	if !strings.Contains(out, "STREAM-003") {
		t.Fatalf("expected STREAM-003 finding from the real file content via files map: %s", out)
	}
}

func TestReview_FilesMapUnreadablePathReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.go")
	pathJSON, _ := json.Marshal(missing)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"violation.go":` + string(pathJSON) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, "cannot read") || !strings.Contains(out, missing) {
		t.Fatalf("expected an explicit cannot-read error naming the path, got: %s", out)
	}
}

func TestReview_FilesMapEmptyShapeReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"a.go":123}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("expected an explicit empty-decode error, got a generic parse error: %s", out)
	}
	if !strings.Contains(out, "empty source after decode") {
		t.Fatalf("expected explicit empty-decode error, got: %s", out)
	}
}

func TestReview_KindDirectoryMergesFiles(t *testing.T) {
	bin := buildMCP(t)
	dir := t.TempDir()
	a := `package a
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) { out.Plan("a") }
`
	b := `package b
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) { out.Changes("b") }
`
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(a), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	dirJSON, _ := json.Marshal(dir)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"directory","directory":` + string(dirJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, "API-032") {
		t.Fatalf("directory review missing API-032: %s", out)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Fatalf("directory review must merge both files: %s", out)
	}
}

func TestReview_PackageKindHonorsDesiredVersion(t *testing.T) {
	bin := buildMCP(t)
	src, _ := json.Marshal("package p\nimport evo \"github.com/zachbornheimer/evident-output\"\nfunc f(g *evo.GroupHandle, p string) {\n\tg.Task(p).Delete(\"worktree\", func() error { return nil })\n}\n")
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","desired_version":"v1.0.0","files":{"prune.go":` + string(src) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "API-032") {
		t.Fatalf("package kind at desired_version=v1.0.0 must not fire 1.1-only API-032: %s", out)
	}
}

// A clean multi-file package ends the MUST-loop through the MCP tool:
// imports are never loaded, and that alone must not force a recheck.
// TestReview_InlineSourceNoAbsoluteFileReportsPartial pins the review-gap
// report's BLOCKER: kind=go (the default) with inline `source` and no
// absolute `file` cannot resolve a module, so it can never evaluate
// API-070/090/091/120 (removed-name) findings — the same call site the
// AGENTS.md MUST-loop drives clean must say Partial=true instead of
// silently reporting a clean result an agent would stop looping on.
func TestReview_InlineSourceNoAbsoluteFileReportsPartial(t *testing.T) {
	bin := buildMCP(t)
	src, _ := json.Marshal(`package p

import evo "github.com/zachbornheimer/evident-output"

func run(t *evo.TaskHandle) {
	t.Warn("tool version differs from manifest")
}
`)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"go","file":"p.go","source":` + string(src) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, `"partial":true`) {
		t.Fatalf("inline source with no absolute file must report partial:true (API-070/090/091/120 unevaluated): %s", out)
	}
}

// TestReview_PackageKindReportsRemovedNameFindings pins the review-gap
// report's BLOCKER: b02b031 wired API-070/090/091/120 (removed-name)
// findings into GoDirectoryAt/GoFileAt via fix.RemovedNameAnalyzers and
// deleted review_warn.go, the only detector that used to cover kind=package
// too, but never wired kind=package (cmd/evident-output-mcp/tools_review.go
// still calls review.GoPackageAt directly) into the replacement. An agent
// running the AGENTS.md MUST-loop on kind=package would see this Warn call
// as a false "findings=0 ... clean" instead of the API-070 finding the
// same source gets under kind=go or kind=directory.
func TestReview_PackageKindReportsRemovedNameFindings(t *testing.T) {
	bin := buildMCP(t)
	src, _ := json.Marshal(`package p

import evo "github.com/zachbornheimer/evident-output"

func run(t *evo.TaskHandle) { t.Warn("tool version differs") }
`)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"p.go":` + string(src) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, "API-070") {
		t.Fatalf("kind=package must report API-070 for a removed Warn call, same as kind=go/kind=directory: %s", out)
	}
	if !strings.Contains(out, `"recheck_required":true`) {
		t.Fatalf("kind=package with a removed-name finding must require a recheck: %s", out)
	}
}

func TestReview_PackageKindCleanPackageIsClean(t *testing.T) {
	bin := buildMCP(t)
	mainSrc, _ := json.Marshal("package main\n\nimport (\n\t\"os\"\n\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n\nfunc main() {\n\tevo.Init(evo.Config{Title: \"tool\"})\n\tos.Exit(evo.Main(run))\n}\n")
	runSrc, _ := json.Marshal("package main\n\nimport (\n\t\"context\"\n\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n\nfunc run(ctx context.Context) error {\n\tevo.Task(\"check config\").Define(func() error { return ctx.Err() })\n\treturn nil\n}\n")
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"main.go":` + string(mainSrc) + `,"run.go":` + string(runSrc) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, "findings=0 recheck=false partial=false") {
		t.Fatalf("clean package must review clean: %s", out)
	}
}
