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

const isolatedInitThenInventory = `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  return err
}
`

const isolatedInitTaskThenInventory = `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  inv := planning.Task("inventory")
  inv.Doing("walking worktrees")
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  return err
}
`

const isolatedInitThenRange = `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  for _, w := range worktrees {
    _ = w
  }
  inv := planning.Task("inventory")
  inv.Doing("walking worktrees")
  return nil
}
`

const isolatedInitTaskThenRange = `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  inv := planning.Task("inventory")
  inv.Doing("walking worktrees")
  for _, w := range worktrees {
    _ = w
  }
  return nil
}
`

const evoCallSoup = `package app
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output, a, b, c string) {
  out.Task(fmt.Sprintf("%s-%s-%s", a, b, c))
}
`

const evoInlineMake = `package app
import evo "github.com/zachbornheimer/evident-output"
func run(g *evo.GroupHandle, n int) {
  for _, t := range g.Each(make([]string, n)) {
    _ = t
  }
}
`

const evoInlineNew = `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  evo.Init(evo.Config{Title: "t", Stderr: new(bytes.Buffer)})
}
`

const evoNamedLocals = `package app
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output, a, b, c string, n int, g *evo.GroupHandle) {
  name := fmt.Sprintf("%s-%s-%s", a, b, c)
  out.Task(name)
  items := make([]string, n)
  for _, t := range g.Each(items) {
    _ = t
  }
}
`

func TestReview_IsolatedInitThenInventoryIsFP002(t *testing.T) {
	out := mcpReviewSource(t, isolatedInitThenInventory)
	if !strings.Contains(out, "FP-002") {
		t.Fatalf("expected FP-002 on Isolated Init then Inventory: %s", out)
	}
}

func TestReview_TaskBeforeInventoryIsNotFP002(t *testing.T) {
	out := mcpReviewSource(t, isolatedInitTaskThenInventory)
	if strings.Contains(out, "FP-002") {
		t.Fatalf("false positive FP-002 when Task is declared before Inventory: %s", out)
	}
}

func TestReview_IsolatedInitThenRangeIsFP002(t *testing.T) {
	out := mcpReviewSource(t, isolatedInitThenRange)
	if !strings.Contains(out, "FP-002") {
		t.Fatalf("expected FP-002 on Isolated Init then for/range before Task: %s", out)
	}
}

func TestReview_TaskBeforeRangeIsNotFP002(t *testing.T) {
	out := mcpReviewSource(t, isolatedInitTaskThenRange)
	if strings.Contains(out, "FP-002") {
		t.Fatalf("false positive FP-002 when Task is declared before the loop: %s", out)
	}
}

func TestReview_MultiArgCallSoupAtEvoSiteIsAPI045(t *testing.T) {
	out := mcpReviewSource(t, evoCallSoup)
	if !strings.Contains(out, "API-045") {
		t.Fatalf("expected API-045 on multi-arg call soup at Task: %s", out)
	}
}

func TestReview_InlineMakeAtEvoSiteIsAPI045(t *testing.T) {
	out := mcpReviewSource(t, evoInlineMake)
	if !strings.Contains(out, "API-045") {
		t.Fatalf("expected API-045 on inline make at Each: %s", out)
	}
}

func TestReview_InlineNewAtEvoSiteIsAPI045(t *testing.T) {
	out := mcpReviewSource(t, evoInlineNew)
	if !strings.Contains(out, "API-045") {
		t.Fatalf("expected API-045 on inline new at Init: %s", out)
	}
}

func TestReview_NamedLocalsBeforeEvoCallAreClean(t *testing.T) {
	out := mcpReviewSource(t, evoNamedLocals)
	if strings.Contains(out, "API-045") {
		t.Fatalf("false positive API-045 when named locals are extracted before the call: %s", out)
	}
}

func mcpReviewSource(t *testing.T, src string) string {
	t.Helper()
	bin := buildMCP(t)
	path := filepath.Join(t.TempDir(), "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	pathJSON, _ := json.Marshal(path)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":` + string(pathJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("review fixture did not parse: %s", out)
	}
	return out
}
