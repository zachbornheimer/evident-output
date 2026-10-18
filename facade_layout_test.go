package evo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	rootOutputAliasDecl  = regexp.MustCompile(`(?m)^type Output = `)
	rootOutputStructDecl = regexp.MustCompile(`(?m)^type Run struct\b`)
)

// TestFacade_RootOutputIsWrapperNotAlias locks the public surface: Output is
// declared in evo so engine test helpers cannot leak through a type alias.
func TestFacade_RootOutputIsWrapperNotAlias(t *testing.T) {
	root := moduleRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatalf("glob root *.go: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no root *.go files")
	}
	var aliases, structs []string
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		if rootOutputAliasDecl.Match(body) {
			aliases = append(aliases, filepath.Base(path))
		}
		if rootOutputStructDecl.Match(body) {
			structs = append(structs, filepath.Base(path))
		}
	}
	if len(aliases) > 0 {
		t.Fatalf("root Output must be a wrapper, not an alias; found type Output = in %s", strings.Join(aliases, ", "))
	}
	if len(structs) == 0 {
		t.Fatal("root Output must be declared as a struct wrapper")
	}
}

func TestFacade_EngineDoesNotImportRoot(t *testing.T) {
	root := moduleRoot(t)
	engineDir := filepath.Join(root, "internal", "engine")
	if _, err := os.Stat(engineDir); !os.IsNotExist(err) {
		t.Fatalf("internal/engine must not exist after slice 8; stat: %v", err)
	}
}
