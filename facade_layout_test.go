package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// assembled so this file itself does not contain the forbidden declaration text.
var rootOutputStructDecl = regexp.MustCompile(`(?m)^type ` + `Output struct\b`)

// TestFacade_RootHasNoOutputStruct locks the thin-root contract: the public
// package is a facade, and Output's struct is declared once in internal/engine.
func TestFacade_RootHasNoOutputStruct(t *testing.T) {
	root := moduleRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatalf("glob root *.go: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no root *.go files")
	}
	var hits []string
	for _, path := range matches {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		if rootOutputStructDecl.Match(body) {
			hits = append(hits, filepath.Base(path))
		}
	}
	if len(hits) > 0 {
		t.Fatalf("root facade must not declare Output as a struct; found in %s", strings.Join(hits, ", "))
	}
}

func TestFacade_EngineDoesNotImportRoot(t *testing.T) {
	root := moduleRoot(t)
	engineDir := filepath.Join(root, "internal", "engine")
	matches, err := filepath.Glob(filepath.Join(engineDir, "*.go"))
	if err != nil {
		t.Fatalf("glob engine *.go: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no internal/engine *.go files")
	}
	importRoot := regexp.MustCompile(`"github.com/zachbornheimer/evident-output"`)
	var hits []string
	for _, path := range matches {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		if importRoot.Match(body) {
			hits = append(hits, filepath.Base(path))
		}
	}
	if len(hits) > 0 {
		t.Fatalf("internal/engine must not import the root package; found in %s", strings.Join(hits, ", "))
	}
}
