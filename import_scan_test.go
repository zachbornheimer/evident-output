package evo_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// facadePackages are the only internal packages allowed to touch the system.
var facadePackages = []string{
	"internal/terminal",
	"internal/process",
	"internal/fs",
	"internal/clock",
}

// clockPackage is the one facade that reads and waits on the wall clock.
const clockPackage = "internal/clock"

// modulePath prefixes every in-module import path.
const modulePath = "github.com/zachbornheimer/evident-output"

// scanProductionFiles runs inspect on every non-test Go file under root,
// except beneath a directory skip reports true for. It returns the sorted
// findings and how many files it inspected.
func scanProductionFiles(root string, skip func(dir string) bool, inspect func(file string) ([]string, error)) ([]string, int, error) {
	var found []string
	checked := 0
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", p, err)
		}
		slashed := filepath.ToSlash(p)
		if d.IsDir() {
			if skip(slashed) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isProductionGoFile(slashed) {
			return nil
		}
		checked++
		hits, inspectErr := inspect(slashed)
		if inspectErr != nil {
			return fmt.Errorf("inspect %s: %w", slashed, inspectErr)
		}
		found = append(found, hits...)
		return nil
	})
	sort.Strings(found)
	return found, checked, walkErr
}

func isProductionGoFile(slashed string) bool {
	return strings.HasSuffix(slashed, ".go") && !strings.HasSuffix(slashed, "_test.go")
}

func skipNothing(string) bool { return false }

func isFacadePackage(dir string) bool {
	return slices.Contains(facadePackages, dir)
}

// isFacadeImport reports whether importPath is one of the four facade packages.
func isFacadeImport(importPath string) bool {
	return slices.Contains(facadePackages, strings.TrimPrefix(importPath, modulePath+"/"))
}

// isStandardLibrary reports whether importPath is a standard-library import:
// its first element carries no dot.
func isStandardLibrary(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

// isNonClockModuleImport reports whether importPath is another package of
// this module other than internal/clock.
func isNonClockModuleImport(importPath string) bool {
	local, inModule := strings.CutPrefix(importPath, modulePath+"/")
	return inModule && local != clockPackage
}

// isNonFacadeImport reports whether importPath is neither the standard
// library nor a facade.
func isNonFacadeImport(importPath string) bool {
	return !isStandardLibrary(importPath) && !isFacadeImport(importPath)
}

// neverBanned is the import rule that bans no import.
func neverBanned(string) bool { return false }

// touches parses file and reports each import for which banned reports true
// and each reference to a function in calls (import path to function names),
// whether called or taken as a value, as "file:line:col import|call name".
func touches(file string, banned func(importPath string) bool, calls map[string]map[string]bool) ([]string, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	var found []string
	localNames := map[string]string{} // local package name -> import path
	for _, spec := range parsed.Imports {
		importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil {
			return nil, fmt.Errorf("unquote import %s: %w", spec.Path.Value, unquoteErr)
		}
		if banned(importPath) {
			found = append(found, fset.Position(spec.Pos()).String()+" import "+importPath)
		}
		if _, watched := calls[importPath]; watched {
			localNames[localPackageName(spec, importPath)] = importPath
		}
	}
	if len(localNames) == 0 {
		return found, nil
	}
	ast.Inspect(parsed, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, imported := localNames[pkg.Name]
		if imported && calls[importPath][sel.Sel.Name] {
			found = append(found, fset.Position(sel.Pos()).String()+" call "+path.Base(importPath)+"."+sel.Sel.Name)
		}
		return true
	})
	return found, nil
}

// localPackageName is the name a file refers to an import by.
func localPackageName(spec *ast.ImportSpec, importPath string) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	return path.Base(importPath)
}
