package evo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestExportedSentinelsLiveInErrorsGo holds CONTRACT.md §7 to one file: an
// exported Err* variable declared anywhere in the root package outside
// errors.go is a sentinel a reader cannot find where §7 sends them.
func TestExportedSentinelsLiveInErrorsGo(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "errors.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, sentinel := range exportedSentinelsDeclared(file) {
			t.Errorf("%s declares exported sentinel %s; sentinels live in errors.go", name, sentinel)
		}
	}
}

// exportedSentinelsDeclared is every exported Err* package variable in file.
func exportedSentinelsDeclared(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			for _, id := range spec.(*ast.ValueSpec).Names {
				if id.IsExported() && strings.HasPrefix(id.Name, "Err") {
					names = append(names, id.Name)
				}
			}
		}
	}
	return names
}
