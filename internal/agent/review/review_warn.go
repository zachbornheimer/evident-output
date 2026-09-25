// Package review — API-070: TaskHandle.Warn, Output.Warn, and evo.Warn were
// removed in 1.1. Problem wins over Warn. Detected by walking the AST and
// resolving each Warn call's receiver to evo by its declared parameter
// type, a chained evo constructor, or a traced local assignment — never by
// matching the receiver's identifier name against a fixed word list. A
// receiver-name list either misses a real call (a project spells its
// *evo.GroupHandle "branches", "remotes", "cleanup", "worktrees" — none of
// those names are in any fixed list) or never matches a chained call at
// all (`out.Task("x").Warn("y")` has no identifier directly before
// `.Warn(`, so an anchored `\w+\.Warn\(` regex never sees it).
package review

import (
	"go/ast"
	"go/token"
	"go/types"
)

// detectWarnRemoved is API-070.
func detectWarnRemoved(filename string, file *ast.File, fset *token.FileSet) []Finding {
	if file == nil {
		return nil
	}
	alias := evoImportName(file)
	if alias == "" {
		return nil
	}
	evoVars := evoValuedIdents(file, alias)

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Warn" {
			return true
		}
		if !exprIsEvo(sel.X, alias, evoVars) {
			return true
		}
		recv := types.ExprString(sel.X)
		findings = append(findings, Finding{
			RuleID:     "API-070",
			Message:    "Warn was removed in 1.1 — Problem wins over Warn; warning is a Problem severity",
			File:       filename,
			Line:       fset.Position(call.Pos()).Line,
			Suggestion: warnSuggestion(recv, alias),
		})
		return true
	})
	return findings
}

// warnSuggestion mirrors the retiredSpelling table's derived-rewrite shape:
// the package-level alias.Warn(...) call (removed in 1.1) has no
// package-level alias.Problem function (Problem is a type), so its
// replacement goes through the default instance; every other receiver
// rewrites in place. Both branches build the suggestion from alias, never a
// hard-coded "evo.", so a file that imports the package under a different
// name still gets a suggestion it can paste as-is.
func warnSuggestion(recv, alias string) string {
	if recv == alias {
		return `replace ` + alias + `.Warn("summary", opts...) (removed in 1.1) with ` + alias + `.Default().Problem("summary", append(opts, ` + alias + `.Severity(` + alias + `.SeverityWarning))...)`
	}
	return "replace " + recv + `.Warn("summary", opts...) with ` + recv + `.Problem("summary", append(opts, ` + alias + `.Severity(` + alias + `.SeverityWarning))...)`
}

// evoValuedIdents collects every identifier this file assigns a value the
// evo package produced: a function/method parameter typed *evo.X or evo.X,
// and a `:=`/`=` local whose right-hand side is a call chain rooted at the
// evo package or at another already-known evo identifier. It fixed-points
// over a few passes so declaration order within a function does not matter.
func evoValuedIdents(file *ast.File, alias string) map[string]bool {
	known := map[string]bool{}
	for range 4 {
		before := len(known)
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				addEvoParams(node.Type.Params, alias, known)
				if node.Recv != nil && exprTypeIsEvo(fieldsType(node.Recv), alias) {
					for _, name := range node.Recv.List[0].Names {
						known[name.Name] = true
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

// exprTypeIsEvo reports whether a type expression (a parameter or receiver
// type as written in source) names a type from the evo package: evo.X or
// *evo.X.
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

// addEvoAssign marks a single-value `x := rhs` / `x = rhs` local as evo
// when rhs's outermost call resolves to evo via exprIsEvo — this is what
// lets `branches := out.Group("branches")` (and a further
// `child := branches.Task("x")`) register "branches" as evo without ever
// matching its name against a fixed list.
func addEvoAssign(assign *ast.AssignStmt, alias string, known map[string]bool) {
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return
	}
	id, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || id.Name == "_" {
		return
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if exprIsEvo(sel.X, alias, known) {
		known[id.Name] = true
	}
}

// exprIsEvo reports whether a receiver expression is evo-valued: the
// import alias itself (evo.Default(), the removed-in-1.1 evo.Warn(...)), a
// known evo local, or a chained call whose own receiver is evo-valued
// (out.Task("x").Warn(...), also removed in 1.1).
func exprIsEvo(x ast.Expr, alias string, known map[string]bool) bool {
	switch e := x.(type) {
	case *ast.Ident:
		return e.Name == alias || known[e.Name]
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		return exprIsEvo(sel.X, alias, known)
	case *ast.ParenExpr:
		return exprIsEvo(e.X, alias, known)
	default:
		return false
	}
}
