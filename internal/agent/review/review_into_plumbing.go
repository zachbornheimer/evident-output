// Package review — API-067 (ZYS-1368): `.Into(&x)` result plumbing on a
// Task. Evo hands values between Tasks through evo.Compute and
// After(computed), never by writing into a caller pointer (contract §31).
package review

import (
	"go/ast"
	"go/token"
)

// detectIntoResultPlumbing is API-067.
func detectIntoResultPlumbing(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isIntoAddressCall(call) {
			return true
		}
		if _, onHandle := parseHandleChain(call.Fun.(*ast.SelectorExpr).X); !onHandle {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-067",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Message:    "Into(&...) plumbs a Task result through a caller pointer: Evo has no such hook, and the value has no stated order or type check",
			Suggestion: "use value := evo.Compute(task, func(ctx context.Context) (T, error) { ... }); consumers declare After(value) and read value.Get()",
		})
		return true
	})
	return findings
}

// isIntoAddressCall is x.Into(&y).
func isIntoAddressCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Into" || len(call.Args) != 1 {
		return false
	}
	addr, ok := call.Args[0].(*ast.UnaryExpr)
	return ok && addr.Op == token.AND
}
