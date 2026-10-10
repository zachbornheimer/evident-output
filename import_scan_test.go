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

// misusePackage is where misuse codes and remedies live; graph may report
// into it once it exists.
const misusePackage = "internal/misuse"

// isGraphForbiddenImport reports whether importPath is a package of this
// module other than record, misuse and the facades: the only packages
// internal/graph may import.
func isGraphForbiddenImport(importPath string) bool {
	local, inModule := strings.CutPrefix(importPath, modulePath+"/")
	if !inModule {
		return false
	}
	return local != recordPackage && local != misusePackage && !isFacadeImport(importPath)
}

// isGraphImport reports whether importPath is internal/graph.
func isGraphImport(importPath string) bool {
	return importPath == modulePath+"/"+graphPackage
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

// recordMutatorPrefixes begin the name of every method that writes run truth
// into package record.
var recordMutatorPrefixes = []string{
	"Record", "Append", "Apply", "Resolve", "Set", "Transition", "Mark", "Clear",
}

// recordHandleName is the field and variable name the producers give a
// record.Run or record.Task handle (o.rec, t.rec, rec).
const recordHandleName = "rec"

func isRecordMutator(method string) bool {
	return slices.ContainsFunc(recordMutatorPrefixes, func(prefix string) bool {
		return strings.HasPrefix(method, prefix)
	})
}

// recordWriteCalls parses file and reports each call of a record-mutating
// method as "file:line:col call Method". See recordWriteCallsIn.
func recordWriteCalls(file string) ([]string, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return recordWriteCallsIn(fset, parsed)
}

// recordWriteCallsIn reports each call x.Method(...) where Method begins with
// one of recordMutatorPrefixes and x is a record value. Without type
// information x counts as a record value when
//   - x, or the field x selects, is declared in the same file with a type
//     from the record package (a parameter, field, result or var of
//     record.X or *record.X, or a variable assigned record.F(...) or
//     &record.X{}), or
//   - x is, or selects a field named, recordHandleName (o.rec, t.rec, rec).
//
// The heuristic is scope-blind (a name declared record-typed anywhere in the
// file counts everywhere in it) and sees only same-file declarations; a
// record value reached through a method result or another file's type is
// missed. That is why the producers keep the recordHandleName convention.
func recordWriteCallsIn(fset *token.FileSet, parsed *ast.File) ([]string, error) {
	pkgName, imported, err := importedRecordName(parsed)
	if err != nil {
		return nil, err
	}
	recordNames := map[string]bool{recordHandleName: true}
	if imported {
		collectRecordTypedNames(parsed, pkgName, recordNames)
	}
	var found []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isRecordMutator(sel.Sel.Name) || !isRecordValue(sel.X, recordNames) {
			return true
		}
		found = append(found, fset.Position(sel.Pos()).String()+" call "+sel.Sel.Name)
		return true
	})
	return found, nil
}

// importedRecordName is the name parsed refers to package record by.
func importedRecordName(parsed *ast.File) (name string, imported bool, err error) {
	for _, spec := range parsed.Imports {
		importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil {
			return "", false, fmt.Errorf("unquote import %s: %w", spec.Path.Value, unquoteErr)
		}
		if importPath == modulePath+"/"+recordPackage {
			return localPackageName(spec, importPath), true, nil
		}
	}
	return "", false, nil
}

// collectRecordTypedNames adds to names every identifier parsed declares with
// a type from the package pkgName refers to record by.
func collectRecordTypedNames(parsed *ast.File, pkgName string, names map[string]bool) {
	ast.Inspect(parsed, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.Field:
			if isRecordType(decl.Type, pkgName) {
				addIdentNames(decl.Names, names)
			}
		case *ast.ValueSpec:
			if decl.Type != nil && isRecordType(decl.Type, pkgName) {
				addIdentNames(decl.Names, names)
			}
		case *ast.AssignStmt:
			for i, rhs := range decl.Rhs {
				if i < len(decl.Lhs) && isRecordConstruction(rhs, pkgName) {
					if lhs, ok := decl.Lhs[i].(*ast.Ident); ok {
						names[lhs.Name] = true
					}
				}
			}
		}
		return true
	})
}

func addIdentNames(idents []*ast.Ident, names map[string]bool) {
	for _, ident := range idents {
		names[ident.Name] = true
	}
}

// isRecordType reports whether expr is record.X or *record.X.
func isRecordType(expr ast.Expr, pkgName string) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	return selectsPackage(expr, pkgName)
}

// isRecordConstruction reports whether expr is record.F(...), &record.X{} or
// record.X{}.
func isRecordConstruction(expr ast.Expr, pkgName string) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		return selectsPackage(e.Fun, pkgName)
	case *ast.UnaryExpr:
		return isRecordConstruction(e.X, pkgName)
	case *ast.CompositeLit:
		return selectsPackage(e.Type, pkgName)
	}
	return false
}

func selectsPackage(expr ast.Expr, pkgName string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkgName
}

// isRecordValue reports whether receiver names a record value: an identifier
// or a selected field whose name is in names.
func isRecordValue(receiver ast.Expr, names map[string]bool) bool {
	switch r := receiver.(type) {
	case *ast.Ident:
		return names[r.Name]
	case *ast.SelectorExpr:
		return names[r.Sel.Name]
	}
	return false
}
