package resource

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestNoPackageFunctionVars keeps filesystem facades off mutable package
// variables: a test that swaps one races every parallel test in the
// process. Inject a resolver value instead.
func TestNoPackageFunctionVars(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				for _, v := range spec.(*ast.ValueSpec).Values {
					if _, isFuncValue := v.(*ast.SelectorExpr); isFuncValue {
						t.Errorf("%s: package var holds a function value; pass it in a value instead", fset.Position(v.Pos()))
					}
				}
			}
		}
	}
}
