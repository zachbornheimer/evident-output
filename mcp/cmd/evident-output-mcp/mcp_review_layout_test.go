package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reviewGoFileViaMCPPath(t *testing.T, relPath, src string) string {
	t.Helper()
	bin := buildMCP(t)
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
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

func TestReview_CleanRepoGoRegisteringUsePruneIsLAYOUT001(t *testing.T) {
	src := `package app
import "github.com/spf13/cobra"
const cleanRepoCommandName = "prune"
func Command() *cobra.Command {
  return &cobra.Command{
    Use: cleanRepoCommandName,
    RunE: func(cmd *cobra.Command, args []string) error { return nil },
  }
}
`
	out := reviewGoFileViaMCPPath(t, "clean_repo.go", src)
	if !strings.Contains(out, "LAYOUT-001") {
		t.Fatalf("expected LAYOUT-001 on clean_repo.go / cleanRepoCommandName Use prune: %s", out)
	}
}

func TestReview_PruneGoWithCleanRepoAliasIsNotLAYOUT001(t *testing.T) {
	src := `package prune
import "github.com/spf13/cobra"
func Command() *cobra.Command {
  return &cobra.Command{
    Use: "prune",
    Aliases: []string{"clean-repo"},
    RunE: func(cmd *cobra.Command, args []string) error { return nil },
  }
}
`
	out := reviewGoFileViaMCPPath(t, "prune.go", src)
	if strings.Contains(out, "LAYOUT-001") {
		t.Fatalf("prune.go Use prune + Aliases clean-repo must not be LAYOUT-001: %s", out)
	}
}

func TestReview_PurgeRunEUnderInternalAppIsLAYOUT002(t *testing.T) {
	src := `package app
import "github.com/spf13/cobra"
func Command() *cobra.Command {
  return &cobra.Command{
    Use: "purge",
    RunE: func(cmd *cobra.Command, args []string) error {
      return walkAndPurge()
    },
  }
}
func walkAndPurge() error { return nil }
`
	out := reviewGoFileViaMCPPath(t, "internal/app/purge.go", src)
	if !strings.Contains(out, "LAYOUT-002") {
		t.Fatalf("expected LAYOUT-002 on purge RunE under internal/app/: %s", out)
	}
}

func TestReview_PurgeOwnedUnderInternalPurgeIsNotLAYOUT002(t *testing.T) {
	src := `package purge
import "github.com/spf13/cobra"
func Command() *cobra.Command {
  return &cobra.Command{
    Use: "purge",
    RunE: func(cmd *cobra.Command, args []string) error {
      return walkAndPurge()
    },
  }
}
func walkAndPurge() error { return nil }
`
	out := reviewGoFileViaMCPPath(t, "internal/purge/purge.go", src)
	if strings.Contains(out, "LAYOUT-002") {
		t.Fatalf("internal/purge owning RunE must not be LAYOUT-002: %s", out)
	}
}

func TestReview_ThinAppDelegateReturningPurgeCommandIsNotLAYOUT002(t *testing.T) {
	src := `package app
import "example.com/tool/internal/purge"
func purgeCommand() *cobra.Command {
  return purge.Command()
}
`
	out := reviewGoFileViaMCPPath(t, "internal/app/app.go", src)
	if strings.Contains(out, "LAYOUT-002") {
		t.Fatalf("thin internal/app return purge.Command() must not be LAYOUT-002: %s", out)
	}
}
