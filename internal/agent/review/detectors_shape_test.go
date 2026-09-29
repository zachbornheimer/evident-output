package review

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestFileDetectorsAreFlatFunctionValues keeps the registry a flat list:
// every entry's run is a detector function, or textRule/astRule of one,
// never an inline closure that re-orders arguments.
func TestFileDetectorsAreFlatFunctionValues(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "detectors.go", nil, 0)
	if err != nil {
		t.Fatalf("parse detectors.go: %v", err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "run" {
			return true
		}
		switch v := kv.Value.(type) {
		case *ast.Ident:
		case *ast.CallExpr:
			if fn, ok := v.Fun.(*ast.Ident); !ok || (fn.Name != "textRule" && fn.Name != "astRule") {
				t.Errorf("%s: run is a call to something other than textRule/astRule", fset.Position(v.Pos()))
			}
		default:
			t.Errorf("%s: run is %T, want a detector function value", fset.Position(v.Pos()), v)
		}
		return true
	})
}
