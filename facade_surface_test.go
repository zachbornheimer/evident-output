package evo_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// leakedSystemTypes are the system types an exported facade signature must
// not name, by import path. A caller holding one could reach past the
// facade's narrow surface (chmod a raw handle, signal a raw process).
var leakedSystemTypes = map[string]map[string]bool{
	"os":      {"File": true, "FileInfo": true, "Signal": true},
	"os/exec": {"Cmd": true},
}

// aliasableSystemTypes are the leaked types a facade may still alias:
// process.Signal stays os.Signal because callers only pass it to Notify and
// StopNotify. Every other leaked type must be wrapped, not aliased.
var aliasableSystemTypes = map[string]map[string]bool{
	"os": {"Signal": true},
}

// surfaceFacades are the facades whose exported surface must not leak a
// system type. internal/clock exposes only time values.
var surfaceFacades = []string{"internal/fs", "internal/process", "internal/terminal"}

// TestFacadeSurfaceLeaksNoSystemTypes fails when an exported func or method of
// internal/fs, internal/process or internal/terminal has a parameter or result
// of type os.File, os.FileInfo, exec.Cmd or os.Signal, or when an exported type
// there is an alias of one of the first three.
func TestFacadeSurfaceLeaksNoSystemTypes(t *testing.T) {
	var leaks []string
	for _, facade := range surfaceFacades {
		found, checked, err := scanProductionFiles(facade, skipNothing, leakedSurface)
		if err != nil {
			t.Fatalf("scan %s: %v", facade, err)
		}
		if checked == 0 {
			t.Fatalf("%s holds no Go files", facade)
		}
		leaks = append(leaks, found...)
	}
	if len(leaks) > 0 {
		t.Errorf("%d exported facade declarations leak a system type:\n  %s",
			len(leaks), strings.Join(leaks, "\n  "))
	}
}

// leakedSurface reports each exported declaration in file that names a leaked
// system type, as "file:line:col Name uses pkg.Type".
func leakedSurface(file string) ([]string, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	importPaths, err := importPathsByLocalName(parsed)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				found = append(found, leaksIn(fset, d.Name.Name, d.Type, importPaths, leakedSystemTypes)...)
			}
		case *ast.GenDecl:
			found = append(found, aliasLeaks(fset, d, importPaths)...)
		}
	}
	return found, nil
}

// aliasLeaks reports each exported type alias in d of a type that must be
// wrapped rather than aliased.
func aliasLeaks(fset *token.FileSet, d *ast.GenDecl, importPaths map[string]string) []string {
	var found []string
	for _, spec := range d.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok || !typeSpec.Assign.IsValid() || !typeSpec.Name.IsExported() {
			continue
		}
		for importPath, names := range leakedSystemTypes {
			wrapped := map[string]bool{}
			for name := range names {
				if !aliasableSystemTypes[importPath][name] {
					wrapped[name] = true
				}
			}
			found = append(found, leaksIn(fset, typeSpec.Name.Name, typeSpec.Type, importPaths, map[string]map[string]bool{importPath: wrapped})...)
		}
	}
	return found
}

// leaksIn reports each selector in node that names a type in leaked.
func leaksIn(fset *token.FileSet, owner string, node ast.Node, importPaths map[string]string, leaked map[string]map[string]bool) []string {
	var found []string
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath := importPaths[pkg.Name]
		if leaked[importPath][sel.Sel.Name] {
			found = append(found, fmt.Sprintf("%s %s uses %s.%s", fset.Position(sel.Pos()), owner, pkg.Name, sel.Sel.Name))
		}
		return true
	})
	return found
}

// importPathsByLocalName maps each name a file refers to an import by to its
// import path.
func importPathsByLocalName(parsed *ast.File) (map[string]string, error) {
	paths := map[string]string{}
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("unquote import %s: %w", spec.Path.Value, err)
		}
		paths[localPackageName(spec, importPath)] = importPath
	}
	return paths, nil
}
