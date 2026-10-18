// Package review — code that runs after an evo.Execute or evo.Main call
// has returned. The run is over by then: no live region is drawn and no
// managed stream is open, so STREAM-003 and EVO-LIVE-001 have nothing to
// protect (a CLI printing its own report of the Result is the normal case).
package review

import (
	"go/ast"
	"go/token"
)

// posRange is a half-open span of source positions.
type posRange struct{ start, end token.Pos }

func (r posRange) contains(p token.Pos) bool { return p >= r.start && p < r.end }

// runEndedRanges are the spans of file that execute only after an
// evo.Execute or evo.Main call, written in the same statement list earlier,
// has returned: every later statement of that list. A call under a go
// statement or inside a function literal is not "returned" at that point,
// so it starts no span.
func runEndedRanges(file *ast.File, pkg string) []posRange {
	var spans []posRange
	ast.Inspect(file, func(n ast.Node) bool {
		var list []ast.Stmt
		switch n := n.(type) {
		case *ast.BlockStmt:
			list = n.List
		case *ast.CaseClause:
			list = n.Body
		case *ast.CommClause:
			list = n.Body
		default:
			return true
		}
		for i, stmt := range list {
			if callsRunToCompletion(stmt, pkg) {
				if i+1 < len(list) {
					spans = append(spans, posRange{list[i+1].Pos(), list[len(list)-1].End()})
				}
				break
			}
		}
		return true
	})
	return spans
}

// callsRunToCompletion reports whether executing stmt calls pkg.Execute or
// pkg.Main and waits for it to return.
func callsRunToCompletion(stmt ast.Stmt, pkg string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt:
			return false
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && isEvoIdent(sel.X, pkg) && (sel.Sel.Name == "Execute" || sel.Sel.Name == "Main") {
				found = true
			}
		}
		return !found
	})
	return found
}
