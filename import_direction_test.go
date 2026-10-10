package evo_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// facadePackages are the only internal packages allowed to touch the system.
var facadePackages = []string{
	"internal/terminal",
	"internal/process",
	"internal/fs",
	"internal/clock",
}

// systemImports are import paths no non-facade internal package may use.
// An entry ending in "/" forbids the whole subtree.
var systemImports = []string{
	"os",
	"os/exec",
	"syscall",
	"golang.org/x/term",
	"golang.org/x/sys/",
}

// wallClockCalls are the time-package functions that read or wait on the wall
// clock. Duration and Time arithmetic stay allowed.
var wallClockCalls = map[string]bool{
	"Now":       true,
	"Since":     true,
	"Until":     true,
	"Sleep":     true,
	"After":     true,
	"AfterFunc": true,
	"NewTimer":  true,
	"NewTicker": true,
	"Tick":      true,
}

// TestOnlyFacadesTouchTheSystem fails when an internal package outside the
// four facades imports os, os/exec, syscall, x/term or x/sys, or calls a
// wall-clock function from package time, called or taken as a value.
func TestOnlyFacadesTouchTheSystem(t *testing.T) {
	var violations []string
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(path)
		if d.IsDir() {
			if isFacadePackage(slashed) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(slashed, ".go") || strings.HasSuffix(slashed, "_test.go") {
			return nil
		}
		found, err := systemTouches(slashed)
		violations = append(violations, found...)
		return err
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Errorf("%d places outside the facades touch the system:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}

func isFacadePackage(dir string) bool {
	return slices.Contains(facadePackages, dir)
}

func isSystemImport(path string) bool {
	for _, banned := range systemImports {
		if path == banned || (strings.HasSuffix(banned, "/") && strings.HasPrefix(path, banned)) {
			return true
		}
	}
	return false
}

// systemTouches parses one file and reports each forbidden import and
// wall-clock call as "file:line:col import|call name".
func systemTouches(file string) ([]string, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return nil, err
	}
	var found []string
	timeName := ""
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if isSystemImport(path) {
			found = append(found, fset.Position(spec.Pos()).String()+" import "+path)
		}
		if path == "time" {
			timeName = "time"
			if spec.Name != nil {
				timeName = spec.Name.Name
			}
		}
	}
	if timeName == "" {
		return found, nil
	}
	ast.Inspect(parsed, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && pkg.Name == timeName && wallClockCalls[sel.Sel.Name] {
			found = append(found, fset.Position(sel.Pos()).String()+" call time."+sel.Sel.Name)
		}
		return true
	})
	return found, nil
}
