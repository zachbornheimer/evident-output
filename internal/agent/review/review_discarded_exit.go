// Package review — EVO-EXIT-002: an evo.Main whose returned exit code is
// discarded.
package review

import (
	"go/ast"
	"go/token"
)

// discardedExitSuggestion is EVO-EXIT-002's one fix.
const discardedExitSuggestion = "evo.Main returns the exit code and never exits itself: write os.Exit(evo.Main(run))"

// detectDiscardedMainCode is EVO-EXIT-002 (E-114): evo.Main returns the
// run's exit code and does not call os.Exit, so `evo.Main(run)` as a bare
// statement, or `_ = evo.Main(run)`, exits 0 after a failed or blocked run.
func detectDiscardedMainCode(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		if call := discardedCall(n); call != nil && isEvoPackageCall(call, pkg, "Main") {
			pos := fset.Position(call.Pos())
			findings = append(findings, Finding{
				RuleID:     "EVO-EXIT-002",
				Message:    "the exit code evo.Main returns is discarded, so a failed or blocked run exits 0",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: discardedExitSuggestion,
			})
		}
		return true
	})
	return findings
}

// discardedCall is the call n evaluates and throws away: an expression
// statement, or a single assignment to the blank identifier.
func discardedCall(n ast.Node) *ast.CallExpr {
	switch s := n.(type) {
	case *ast.ExprStmt:
		call, _ := s.X.(*ast.CallExpr)
		return call
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 || identName(s.Lhs[0]) != "_" {
			return nil
		}
		call, _ := s.Rhs[0].(*ast.CallExpr)
		return call
	}
	return nil
}

// isEvoPackageCall reports whether call is pkg.name(...).
func isEvoPackageCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name && isEvoIdent(sel.X, pkg)
}
