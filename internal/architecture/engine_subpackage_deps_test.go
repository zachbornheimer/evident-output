package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// modulePath is this module's import path, read once from go.mod.
const modulePath = "github.com/zachbornheimer/evident-output"

// allowedEngineSubpackageImports lists the only within-module import paths a
// package under internal/engine/ (or internal/ordered) may use. Anything
// else under the module path is a layering violation: these packages exist
// so engine can depend on them, never the reverse.
var allowedEngineSubpackageImports = map[string]bool{
	modulePath + "/internal/core":    true,
	modulePath + "/internal/text":    true,
	modulePath + "/internal/wire":    true,
	modulePath + "/internal/ordered": true,
}

// TestEngineSubpackagesOnlyImportLeaves parses every non-test .go file under
// internal/engine/<subpackage> and internal/ordered, and fails naming any
// import that reaches back into internal/engine itself, a sibling engine
// subpackage, or any other internal package not in the allow-list. It also
// fails any "sync"/"sync/atomic" import outside internal/engine/transcript,
// which is the one package allowed a mutex (see design doc §2.4).
func TestEngineSubpackagesOnlyImportLeaves(t *testing.T) {
	root := moduleRoot(t)
	engineDir := filepath.Join(root, "internal", "engine")
	dirs := []string{filepath.Join(root, "internal", "ordered")}

	entries, err := os.ReadDir(engineDir)
	if err != nil {
		t.Fatalf("reading %s: %v", engineDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(engineDir, e.Name()))
		}
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			continue // not created yet
		}
		checkDir(t, root, dir)
	}
}

func checkDir(t *testing.T, root, dir string) {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	pkgUnderTest := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(dir, root), "/"))
	allowSync := pkgUnderTest == "internal/engine/transcript"

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, f.Name())
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		var bad []string
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if (importPath == "sync" || importPath == "sync/atomic") && !allowSync {
				bad = append(bad, importPath)
				continue
			}
			if strings.HasPrefix(importPath, modulePath+"/") && !allowedEngineSubpackageImports[importPath] {
				bad = append(bad, importPath)
			}
		}
		sort.Strings(bad)
		for _, imp := range bad {
			t.Errorf("%s: disallowed import %q", filepath.Join(pkgUnderTest, f.Name()), imp)
		}
	}
}
