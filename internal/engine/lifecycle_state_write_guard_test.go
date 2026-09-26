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

// guardedSelectors names the taskState/scheduler fields that must be
// written only through their own type's methods, never whole-value
// replaced by a plain AssignStmt: "state" (lifecycle.State — Declared,
// StartRunning, Settle), "standing" (schedule.Standing — Board.Move) and
// "board" (schedule.Board — Board.Move). Each is an ordinary
// engine-package field of an unexported-field type, so the compiler alone
// does not stop `st.sched.standing = schedule.Standing{}` the way it stops
// a write inside package schedule or package lifecycle.
var guardedSelectors = map[string]bool{
	"state":    true,
	"standing": true,
	"board":    true,
}

// TestNoDirectTaskStateAssignment closes the compile-time gap plain
// unexported fields leave open: lifecycle.State's own fields (current,
// settled) cannot be touched outside package lifecycle, but taskState.state
// is an ordinary engine-package field of type lifecycle.State, so engine
// code can still whole-value replace it (`st.state = lifecycle.State{}`,
// or any other lifecycle.State-typed expression) and bypass Declared,
// StartRunning and Settle entirely — the same shape as the constructor
// bypass ZYS-1190 review flagged. schedule.Standing and schedule.Board are
// the same shape: only Board.Move may write a Standing's phase or a
// Board's parked count. Only a composite literal's key at a taskState's
// construction site is legitimate; every other write goes through the
// owning type's own methods. This test walks internal/engine's own source
// (not _test.go files, which may build fixtures) and fails, naming
// file:line, the moment any AssignStmt targets a selector in
// guardedSelectors. Today's legitimate construction sites are all
// composite literals seeding lifecycle.Declared(): declareTaskLocked
// (declare.go) and the synthetic Fail/Cancel paths in run_outcome.go; an
// AssignStmt anywhere is exactly the bypass this test exists to catch. It
// does not (yet) catch a composite literal that copies another Task's
// already-settled state (`taskState{state: other.state}`) or a write
// through a pointer alias of a guarded field — no such code exists today,
// so that is a known gap in coverage, not a live bypass.
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
				if !ok || !guardedSelectors[sel.Sel.Name] {
					continue
				}
				pos := fset.Position(sel.Pos())
				violations = append(violations, filepath.Base(pos.Filename)+":"+strconv.Itoa(pos.Line)+" ("+sel.Sel.Name+")")
			}
			return true
		})
	}

	if len(violations) > 0 {
		t.Fatalf("direct assignment to a guarded field found outside its composite-literal construction site (bypasses the owning type's methods): %v", violations)
	}
}
