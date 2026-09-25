// Package review — API-063: a Verify callback that returns a constant
// claims the Task's desired state holds (true) or never holds (false)
// without observing anything. true is the retired Done stamp under
// another name: the row reads already-satisfied and the callback never
// runs. The callback is found inline or through a local variable bound to
// a function literal.
package review

import (
	"go/ast"
	"go/token"
)

// detectConstantVerify is API-063.
func detectConstantVerify(filename string, file *ast.File, fset *token.FileSet) []Finding {
	lits := localFuncLits(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Verify" {
			return true
		}
		lit := funcLitOf(call.Args[0], lits)
		if lit == nil {
			return true
		}
		if value, ok := constantVerdict(lit); ok {
			findings = append(findings, Finding{
				RuleID:     "API-063",
				Message:    "Verify callback returns a constant " + value + ": it observes nothing, so the Task's evidence is invented",
				File:       filename,
				Line:       fset.Position(call.Pos()).Line,
				Suggestion: "observe the desired state in the Verify callback (stat the file, query the service) and return what you saw; with nothing to check, drop Verify and let Define run",
			})
		}
		return true
	})
	return findings
}

// localFuncLits maps each identifier bound by := to a function literal.
func localFuncLits(file *ast.File) map[string]*ast.FuncLit {
	lits := map[string]*ast.FuncLit{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			id, isIdent := lhs.(*ast.Ident)
			lit, isLit := assign.Rhs[i].(*ast.FuncLit)
			if isIdent && isLit {
				lits[id.Name] = lit
			}
		}
		return true
	})
	return lits
}

func funcLitOf(arg ast.Expr, lits map[string]*ast.FuncLit) *ast.FuncLit {
	switch a := arg.(type) {
	case *ast.FuncLit:
		return a
	case *ast.Ident:
		return lits[a.Name]
	default:
		return nil
	}
}

// constantVerdict reports the literal a (bool, error) callback returns
// when its whole body is one return of true or false.
func constantVerdict(lit *ast.FuncLit) (string, bool) {
	if lit.Type.Results == nil || len(lit.Type.Results.List) != 2 || len(lit.Body.List) != 1 {
		return "", false
	}
	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 2 {
		return "", false
	}
	id, ok := ret.Results[0].(*ast.Ident)
	if !ok || (id.Name != "true" && id.Name != "false") {
		return "", false
	}
	return id.Name, true
}
