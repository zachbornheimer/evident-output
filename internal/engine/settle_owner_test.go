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

// TestOnlySettleLockedEndsATask proves settleLocked is the single owner of
// terminal transitions: no other production code in this package may assign
// a Task's state, except the one non-terminal move to Running. A new
// terminal path that sets st.state by hand would skip the bookkeeping every
// terminal transition owes (waking waiters, releasing a parked Sequence
// step, marking the cascade due) and fails here instead of in production.
func TestOnlySettleLockedEndsATask(t *testing.T) {
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
			if !ok || fn.Body == nil || fn.Name.Name == "settleLocked" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, lhs := range assign.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "state" {
						continue
					}
					if rhs, ok := assign.Rhs[i].(*ast.Ident); ok && rhs.Name == "Running" {
						continue
					}
					t.Errorf("%s: %s assigns a Task's state directly; route the transition through settleLocked",
						fset.Position(assign.Pos()), fn.Name.Name)
				}
				return true
			})
		}
	}
}
