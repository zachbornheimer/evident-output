package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestNoDirectTaskStateAssignment closes the compile-time gap plain
// unexported fields leave open: lifecycle.State's own fields (current,
// settled) cannot be touched outside package lifecycle, but taskState.state
// is an ordinary engine-package field of type lifecycle.State, so engine
// code can still whole-value replace it (`st.state = lifecycle.State{}`,
// or any other lifecycle.State-typed expression) and bypass Declared,
// StartRunning and Settle entirely — the same shape as the constructor
// bypass ZYS-1190 review flagged. Only a composite literal's `state:` key
// at a taskState's construction site is legitimate; every other write goes
// through State's own methods. This test walks internal/engine's own
// source (not _test.go files, which may build fixtures) and fails, naming
// file:line, the moment any AssignStmt targets a `.state` selector. Today's
// legitimate construction sites are all composite literals seeding
// lifecycle.Declared(): declareTaskLocked (declare.go) and the synthetic
// Fail/Cancel paths in run_outcome.go; an AssignStmt anywhere is exactly
// the bypass this test exists to catch. It does not (yet) catch a
// composite literal that copies another Task's already-settled state
// (`taskState{state: other.state}`) or a write through a pointer alias of
// state — no such code exists today, so that is a known gap in coverage,
// not a live bypass.
func TestNoDirectTaskStateAssignment(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}

	fset := token.NewFileSet()
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || len(name) > 8 && name[len(name)-8:] == "_test.go" {
			continue
		}
		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "state" {
					continue
				}
				pos := fset.Position(sel.Pos())
				violations = append(violations, filepath.Base(pos.Filename)+":"+strconv.Itoa(pos.Line))
			}
			return true
		})
	}

	if len(violations) > 0 {
		t.Fatalf("direct assignment to a taskState.state field found outside its composite-literal construction site (bypasses lifecycle.State's Declared/StartRunning/Settle): %v", violations)
	}
}
