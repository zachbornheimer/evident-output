package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLockedSuffix_MeansCallerHoldsMutex enforces this package's naming
// convention mechanically: a function whose name ends in "Locked" requires
// its caller to hold o.mu, so it must never take a .mu lock itself. A
// helper that inverts the convention invites a caller already holding the
// non-reentrant mutex to self-deadlock.
func TestLockedSuffix_MeansCallerHoldsMutex(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasSuffix(fn.Name.Name, "Locked") {
				continue
			}
			if locksMutex(fn.Body) {
				t.Errorf("%s: %s takes .mu itself; drop the Locked suffix or the lock", fset.Position(fn.Pos()), fn.Name.Name)
			}
		}
	}
}

// locksMutex reports whether body itself calls <x>.mu.Lock() or
// <x>.mu.RLock(). A function literal inside body is a callback that runs
// later, outside the caller's critical section, so it is not descended.
func locksMutex(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Lock" && sel.Sel.Name != "RLock") {
			return !found
		}
		if mu, ok := sel.X.(*ast.SelectorExpr); ok && mu.Sel.Name == "mu" {
			found = true
		}
		return !found
	})
	return found
}
