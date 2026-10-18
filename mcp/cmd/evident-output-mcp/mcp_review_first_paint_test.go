package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reviewGoFileViaMCP(t *testing.T, src string) string {
	t.Helper()
	bin := buildMCP(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	pathJSON, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":` + string(pathJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("fixture did not parse: %s", out)
	}
	return out
}

func TestReview_InventoryBeforeFirstTaskReturnsFP002(t *testing.T) {
	src := `package app
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run) error {
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  r.Task("inventory").Define(func(ctx context.Context) error { return err })
  return nil
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "FP-002") {
		t.Fatalf("expected FP-002 on Inventory before the first Task: %s", out)
	}
}

func TestReview_WalkDirLoopBeforeFirstTaskReturnsLOOP001(t *testing.T) {
	src := `package app
import (
  "context"
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run, roots []string) error {
  for _, root := range roots {
    _ = filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil })
  }
  r.Task("inventory").Define(func(ctx context.Context) error { return nil })
  return nil
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "LOOP-001") {
		t.Fatalf("expected LOOP-001 on a for/range WalkDir before the first Task (FP-002 is not a substitute): %s", out)
	}
}

func TestReview_TaskDefineBeforeWorkDoesNotReturnFP002OrLOOP001(t *testing.T) {
	src := `package app
import (
  "context"
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run, roots []string) error {
  inv := r.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func(ctx context.Context) error {
    for _, root := range roots {
      if err := filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil }); err != nil {
        return err
      }
    }
    _, err := purge.Inventory(ctx, opt)
    return err
  })
  return nil
}
`
	out := reviewGoFileViaMCP(t, src)
	if strings.Contains(out, "FP-002") || strings.Contains(out, "LOOP-001") {
		t.Fatalf("Task/Doing/Define before work must not return FP-002 or LOOP-001: %s", out)
	}
}

func TestReview_InlineMakeOrNewInEvoArgIsDirty(t *testing.T) {
	src := `package app
import (
  "bytes"
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func capture(ctx context.Context) int {
  return evo.Execute(ctx, evo.Config{Facades: evo.Facades{Stdout: new(bytes.Buffer)}}, run).ExitCode()
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "CALL-001") {
		t.Fatalf("expected CALL-001 on inline new inside evo.Execute argument: %s", out)
	}
}

func TestReview_NamedLocalBeforeEvoCallIsClean(t *testing.T) {
	src := `package app
import (
  "bytes"
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func capture(ctx context.Context) int {
  stdout := new(bytes.Buffer)
  cfg := evo.Config{Facades: evo.Facades{Stdout: stdout}}
  return evo.Execute(ctx, cfg, run).ExitCode()
}
`
	out := reviewGoFileViaMCP(t, src)
	if strings.Contains(out, "CALL-001") {
		t.Fatalf("an extracted named local before evo.Execute must be clean: %s", out)
	}
}
