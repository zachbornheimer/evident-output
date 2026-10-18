// Package review — DOM-011: a question the user answered no to is Refused,
// not a Go application error. `if !r.Confirm(...) { return errors.New(...) }`
// turns an expected, successfully evaluated answer into a Failed run (exit
// 2) when the contract maps it to Refused (exit 1).
package review

import (
	"go/ast"
	"go/token"
)

// errorConstructors are the standard-library calls that build a plain Go
// error from text.
var errorConstructors = map[string]bool{"errors.New": true, "fmt.Errorf": true, "errors.Join": true}

// detectDeclinedConfirmAsError is DOM-011.
func detectDeclinedConfirmAsError(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	recvs := newEvoHandleReceivers(file, pkg)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok || !isDeclinedConfirm(stmt.Cond, recvs) {
			return true
		}
		if ret := firstReturnOfNewError(stmt.Body); ret != nil {
			findings = append(findings, declinedConfirmFinding(filename, fset.Position(ret.Pos()), pkg))
		}
		return true
	})
	return findings
}

// isDeclinedConfirm reports whether cond is `!x.Confirm(...)` on an evo Run.
func isDeclinedConfirm(cond ast.Expr, recvs evoHandleReceivers) bool {
	not, ok := cond.(*ast.UnaryExpr)
	if !ok || not.Op != token.NOT {
		return false
	}
	call, ok := not.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Confirm" && recvs.Holds(sel.X)
}

// firstReturnOfNewError is the first statement in body that returns an error
// built by errors.New, fmt.Errorf or errors.Join; nested function literals
// are their own functions and are not read.
func firstReturnOfNewError(body *ast.BlockStmt) *ast.ReturnStmt {
	var found *ast.ReturnStmt
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			if call, ok := result.(*ast.CallExpr); ok && errorConstructors[calledFuncDotted(call)] {
				found = ret
			}
		}
		return true
	})
	return found
}

func declinedConfirmFinding(filename string, pos token.Position, pkg string) Finding {
	return Finding{
		RuleID:     "DOM-011",
		Message:    "a declined Confirm is returned as an application error; the user's answer is an expected outcome, so the run should end Refused (exit 1), not Failed (exit 2)",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "replace this return with `return " + pkg + ".Refuse(\"declined\")`; the process status follows the Conclusion (Result.ExitCode())",
	}
}
