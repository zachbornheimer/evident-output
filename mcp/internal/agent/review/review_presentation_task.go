// Package review — API-071 (ZYS-1368): a Task whose whole job is to print.
// A header, separator or summary line is presentation Evo renders itself
// from Group/Sequence names, Facts, and the Conclusion; a Task that only
// prints adds a fake row of work and races the live renderer.
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// detectPresentationOnlyTask is API-071.
func detectPresentationOnlyTask(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	for _, cb := range defineCallbacks(file, evoImportName(file)) {
		if cb.builder || !onlyPrints(cb.lit.Body) {
			continue
		}
		pos := fset.Position(cb.lit.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-071",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Message:    "presentation-only Task: its callback only prints, so the row reports work that never happened",
			Suggestion: "delete the Task; name the Group or Sequence for the phase, record information with task.Fact/Detail on the Task that produced it, and let the Conclusion summarize",
		})
	}
	return findings
}

// onlyPrints reports whether every statement is a print call or a bare
// return of nil, with at least one print.
func onlyPrints(body *ast.BlockStmt) bool {
	prints := 0
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.ExprStmt:
			call, ok := s.X.(*ast.CallExpr)
			if !ok || !isPrintCall(call) {
				return false
			}
			prints++
		case *ast.ReturnStmt:
			if !returnsNilOnly(s) {
				return false
			}
		default:
			return false
		}
	}
	return prints > 0
}

func returnsNilOnly(ret *ast.ReturnStmt) bool {
	return len(ret.Results) == 1 && identName(ret.Results[0]) == "nil"
}

// isPrintCall is any pkg.Print*/Fprint* or handle.Print* call.
func isPrintCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	return strings.HasPrefix(name, "Print") || strings.HasPrefix(name, "Fprint")
}
