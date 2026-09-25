package fix

import "go/ast"

// evoValuedIdents collects every identifier a file assigns a value that
// chains back to the evo package: a parameter typed evo.X/*evo.X, and a
// local whose right-hand side is a call chain rooted at the evo import
// alias or another already-known evo local. It fixed-points over a few
// passes so declaration order within a function does not matter.
//
// This exists only as the fallback path for a name go/types cannot
// resolve at all — one already fully removed from the evo package, so the
// call site is a hard compile error rather than a type mismatch and
// TypesInfo has no Uses entry to walk. It is the same tracing
// internal/agent/review's evoValuedIdents already does for exactly that
// reason, adapted here rather than duplicated with different behavior.
func evoValuedIdents(file *ast.File, alias string) map[string]bool {
	known := map[string]bool{}
	for range 4 {
		before := len(known)
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				addEvoParams(node.Type.Params, alias, known)
				if node.Recv != nil {
					if t := fieldsType(node.Recv); exprTypeIsEvo(t, alias) {
						for _, name := range node.Recv.List[0].Names {
							known[name.Name] = true
						}
					}
				}
			case *ast.FuncLit:
				addEvoParams(node.Type.Params, alias, known)
			case *ast.AssignStmt:
				addEvoAssign(node, alias, known)
			}
			return true
		})
		if len(known) == before {
			break
		}
	}
	return known
}

func fieldsType(fl *ast.FieldList) ast.Expr {
	if fl == nil || len(fl.List) == 0 {
		return nil
	}
	return fl.List[0].Type
}

func addEvoParams(params *ast.FieldList, alias string, known map[string]bool) {
	if params == nil {
		return
	}
	for _, f := range params.List {
		if !exprTypeIsEvo(f.Type, alias) {
			continue
		}
		for _, name := range f.Names {
			known[name.Name] = true
		}
	}
}

func exprTypeIsEvo(t ast.Expr, alias string) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == alias
}

func addEvoAssign(assign *ast.AssignStmt, alias string, known map[string]bool) {
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return
	}
	id, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || id.Name == "_" {
		return
	}
	if aliasReceiver(alias, assign.Rhs[0], known) {
		known[id.Name] = true
	}
}
