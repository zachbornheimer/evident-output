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

func TestReview_IsolatedInitThenInventoryReturnsFP002(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  return err
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "FP-002") {
		t.Fatalf("expected FP-002 on Isolated Init then Inventory: %s", out)
	}
}

func TestReview_IsolatedInitThenWalkDirLoopReturnsLOOP001(t *testing.T) {
	src := `package app
import (
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output"
)
func previewPurge(roots []string) error {
  out := evo.Init(evo.Config{Isolated: true, DryRun: true})
  defer out.Close()
  for _, root := range roots {
    _ = filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil })
  }
  out.Task("inventory").Done()
  return nil
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "LOOP-001") {
		t.Fatalf("expected LOOP-001 on Isolated Init then for/range WalkDir (FP-002 is not a substitute): %s", out)
	}
}

func TestReview_TaskDefineBeforeWorkDoesNotReturnFP002OrLOOP001(t *testing.T) {
	src := `package app
import (
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output"
)
func previewPurge(roots []string) error {
  out := evo.Init(evo.Config{Isolated: true, DryRun: true})
  defer out.Close()
  inv := out.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func() error {
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

func TestReview_MultiArgCallSoupAtEvoSiteIsDirty(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  out := evo.Init(evo.Config{Title: "t"})
  _ = out.Task("scan", "extra")
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "API-032") {
		t.Fatalf("expected a DIRTY finding (API-032) on multi-arg evo.Task call soup: %s", out)
	}
}

func TestReview_InlineMakeOrNewInEvoArgIsDirty(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  out := evo.Init(evo.Config{Title: "t", Facts: make([]evo.Fact, 0)})
  _ = out.Group("jobs")
}
`
	out := reviewGoFileViaMCP(t, src)
	if !strings.Contains(out, "CALL-001") {
		t.Fatalf("expected CALL-001 on inline make inside evo.Init argument: %s", out)
	}
}

func TestReview_NamedLocalBeforeEvoCallIsClean(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  facts := make([]evo.Fact, 0)
  cfg := evo.Config{Title: "t", Facts: facts}
  out := evo.Init(cfg)
  _ = out.Task("scan")
  _ = out.Group("jobs")
}
`
	out := reviewGoFileViaMCP(t, src)
	if strings.Contains(out, "CALL-001") || strings.Contains(out, "API-032") {
		t.Fatalf("extracted named locals before evo.Init/Task/Group must be clean: %s", out)
	}
}
