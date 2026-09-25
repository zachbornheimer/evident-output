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
	var findings []Finding
	var scopes []*ast.BlockStmt // enclosing function bodies, innermost last
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				return walkScope(fn.Body, &scopes, visit)
			}
		case *ast.FuncLit:
			return walkScope(fn.Body, &scopes, visit)
		case *ast.CallExpr:
			if lit := verifyCallback(fn, scopes); lit != nil {
				if value, ok := constantVerdict(lit); ok {
					findings = append(findings, Finding{
						RuleID:     "API-063",
						Message:    "Verify callback returns a constant " + value + ": it observes nothing, so the Task's evidence is invented",
						File:       filename,
						Line:       fset.Position(fn.Pos()).Line,
						Suggestion: "observe the desired state in the Verify callback (stat the file, query the service) and return what you saw; with nothing to check, drop Verify and let Define run",
					})
				}
			}
		}
		return true
	}
	ast.Inspect(file, visit)
	return findings
}

// walkScope visits body with it pushed as the innermost function scope,
// and tells the caller's Inspect not to descend again.
func walkScope(body *ast.BlockStmt, scopes *[]*ast.BlockStmt, visit func(ast.Node) bool) bool {
	*scopes = append(*scopes, body)
	for _, stmt := range body.List {
		ast.Inspect(stmt, visit)
	}
	*scopes = (*scopes)[:len(*scopes)-1]
	return false
}

// verifyCallback is the function literal a one-argument Verify call
// passes, inline or through a local it resolves in scope; nil otherwise.
func verifyCallback(call *ast.CallExpr, scopes []*ast.BlockStmt) *ast.FuncLit {
	if len(call.Args) != 1 {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Verify" {
		return nil
	}
	switch a := call.Args[0].(type) {
	case *ast.FuncLit:
		return a
	case *ast.Ident:
		return boundFuncLit(a, scopes)
	default:
		return nil
	}
}

// boundFuncLit resolves id at its use: the last assignment to its name
// before it in the innermost enclosing function that assigns it, outward.
// It is the function literal that assignment binds, or nil when the name
// was last bound to anything else (or not in any enclosing function).
// Nested function literals are not searched: their bindings are out of
// scope at id (E-108).
func boundFuncLit(id *ast.Ident, scopes []*ast.BlockStmt) *ast.FuncLit {
	for i := len(scopes) - 1; i >= 0; i-- {
		if rhs, found := lastAssignment(scopes[i], id); found {
			lit, _ := rhs.(*ast.FuncLit)
			return lit
		}
	}
	return nil
}

// lastAssignment is the right-hand side of the last assignment to id's
// name in body before id, not counting nested function literals.
func lastAssignment(body *ast.BlockStmt, id *ast.Ident) (rhs ast.Expr, found bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil || n.Pos() >= id.Pos() {
			return false
		}
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			if name, ok := lhs.(*ast.Ident); ok && name.Name == id.Name {
				rhs, found = nil, true
				if len(assign.Lhs) == len(assign.Rhs) {
					rhs = assign.Rhs[i]
				}
			}
		}
		return true
	})
	return rhs, found
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
