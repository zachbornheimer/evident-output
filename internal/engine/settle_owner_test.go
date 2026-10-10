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

// TestOnlyTheGraphSettlesATask proves graph.Graph.Settle is the single
// owner of terminal transitions: no engine code moves a Task's record to a
// state except the one non-terminal move to Running, and none assigns a
// state by hand. The engine listener repaints a terminal transition once and
// only because every terminal transition has this one origin; a new path
// that transitioned a record itself would skip the bookkeeping every
// terminal transition owes (waking waiters, releasing a parked Sequence
// step, marking the cascade due) and fails here instead of in production.
func TestOnlyTheGraphSettlesATask(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list engine sources: %v", err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					reportStateAssignments(t, fset, fn.Name.Name, n)
				case *ast.CallExpr:
					reportTerminalTransition(t, fset, fn.Name.Name, n)
				}
				return true
			})
		}
	}
}

func reportStateAssignments(t *testing.T, fset *token.FileSet, fn string, assign *ast.AssignStmt) {
	t.Helper()
	for i, lhs := range assign.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "state" {
			continue
		}
		if rhs, ok := assign.Rhs[i].(*ast.Ident); ok && rhs.Name == "Running" {
			continue
		}
		t.Errorf("%s: %s assigns a Task's state directly; route the transition through the graph's Settle",
			fset.Position(assign.Pos()), fn)
	}
}

func reportTerminalTransition(t *testing.T, fset *token.FileSet, fn string, call *ast.CallExpr) {
	t.Helper()
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Transition" || len(call.Args) != 1 {
		return
	}
	if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Name == "Running" {
		return
	}
	t.Errorf("%s: %s transitions a Task's record to a state other than Running; settle it through the graph",
		fset.Position(call.Pos()), fn)
}
