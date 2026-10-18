// Package review — API-040: evo.Fail, evo.Refuse and evo.Exclude only build
// the error a Define callback or the run body returns. Calling one as a bare
// statement (or assigning it to the blank identifier) throws the ending away,
// so the Task ends with whatever else the function returns.
package review

import (
	"go/ast"
	"go/token"
)

// endingConstructors are the evo functions whose result is the Task's ending.
var endingConstructors = map[string]bool{"Fail": true, "Refuse": true, "Exclude": true}

// detectDiscardedEnding is API-040.
func detectDiscardedEnding(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		if call := discardedEndingCall(n, pkg); call != nil {
			findings = append(findings, discardedEndingFinding(filename, fset.Position(call.Pos()), pkg, call))
		}
		return true
	})
	return findings
}

// discardedEndingCall is the evo ending constructor n calls and throws away:
// a bare statement, or an assignment of every result to the blank identifier.
func discardedEndingCall(n ast.Node, pkg string) *ast.CallExpr {
	switch stmt := n.(type) {
	case *ast.ExprStmt:
		return endingConstructorCall(stmt.X, pkg)
	case *ast.AssignStmt:
		if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 || !isBlankIdent(stmt.Lhs[0]) {
			return nil
		}
		return endingConstructorCall(stmt.Rhs[0], pkg)
	}
	return nil
}

func endingConstructorCall(e ast.Expr, pkg string) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, pkg) || !endingConstructors[sel.Sel.Name] {
		return nil
	}
	return call
}

func isBlankIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

func discardedEndingFinding(filename string, pos token.Position, pkg string, call *ast.CallExpr) Finding {
	verb := call.Fun.(*ast.SelectorExpr).Sel.Name
	return Finding{
		RuleID:     "API-040",
		Message:    pkg + "." + verb + "(...) builds the Task's ending and this call throws it away; the callback ends with whatever it returns instead, so the summary and options never reach the row",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "return the ending: `return " + pkg + "." + verb + "(...)`; to keep an underlying error's text, put it in " + pkg + ".Detail(err.Error())",
	}
}
