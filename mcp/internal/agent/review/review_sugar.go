package review

import (
	"go/ast"
	"go/token"
)

func detectSingletonGroup(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		findings = append(findings, singletonGroupsInFunc(filename, fn.Body, fset)...)
		return false
	})
	return findings
}

func singletonGroupsInFunc(filename string, body *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Task" && isGroupCall(sel.X) {
			pos := fset.Position(sel.Pos())
			findings = append(findings, singletonGroupFinding(filename, pos, "Group"))
		}
		return true
	})

	type groupUse struct {
		line, col int
		tasks     int
		inLoop    bool
	}
	groups := map[string]*groupUse{}

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if !isGroupCall(rhs) {
				continue
			}
			if i >= len(assign.Lhs) {
				continue
			}
			id, ok := assign.Lhs[i].(*ast.Ident)
			if !ok {
				continue
			}
			pos := fset.Position(rhs.Pos())
			groups[id.Name] = &groupUse{line: pos.Line, col: pos.Column}
		}
		return true
	})

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "Task" {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		g, ok := groups[id.Name]
		if !ok {
			return true
		}
		g.tasks++
		if callInsideLoop(body, call) {
			g.inLoop = true
		}
		return true
	})

	for name, g := range groups {
		if g.inLoop || g.tasks != 1 || groupEscapes(body, name) || groupFilledBeyondTasks(body, name) {
			continue
		}
		findings = append(findings, singletonGroupFindingAt(filename, g.line, g.col, name))
	}
	return findings
}

// groupEscapes reports whether this function hands the group back to its
// caller. A constructor cannot be judged on the children it declared —
// whoever receives the group is where the collection actually gets filled —
// so counting only this body's own Task calls would flag every subject
// factory as a one-child group.
func groupEscapes(body *ast.BlockStmt, name string) bool {
	escapes := false
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || escapes {
			return !escapes
		}
		for _, result := range ret.Results {
			ast.Inspect(result, func(inner ast.Node) bool {
				if id, ok := inner.(*ast.Ident); ok && id.Name == name {
					escapes = true
				}
				return !escapes
			})
		}
		return !escapes
	})
	return escapes
}

func isGroupCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Group" && len(call.Args) == 1
}

func callInsideLoop(body *ast.BlockStmt, target *ast.CallExpr) bool {
	var inside bool
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		if n == nil || inside {
			return
		}
		switch s := n.(type) {
		case *ast.ForStmt:
			if nodeContains(s, target) {
				inside = true
			}
		case *ast.RangeStmt:
			if nodeContains(s, target) {
				inside = true
			}
		}
		for _, child := range childrenOf(n) {
			walk(child)
		}
	}
	walk(body)
	return inside
}

func nodeContains(root ast.Node, target ast.Node) bool {
	found := false
	ast.Inspect(root, func(n ast.Node) bool {
		if n == target {
			found = true
			return false
		}
		return true
	})
	return found
}

func childrenOf(n ast.Node) []ast.Node {
	var out []ast.Node
	ast.Inspect(n, func(c ast.Node) bool {
		if c == nil || c == n {
			return true
		}
		out = append(out, c)
		return false
	})
	return out
}

func singletonGroupFinding(filename string, pos token.Position, recv string) Finding {
	return singletonGroupFindingAt(filename, pos.Line, pos.Column, recv)
}

func singletonGroupFindingAt(filename string, line, col int, recv string) Finding {
	suggestion := "use a lone Task instead of a 1-child Group"
	if recv != "" && recv != "Group" {
		suggestion = "replace " + recv + " with a lone Task declared on the same parent (r.Task(...)); a Group of one child is a single row"
	}
	return Finding{
		RuleID:     "API-039",
		Message:    "Group has exactly one child; it renders as a 0/1 complete header over a single row",
		File:       filename,
		Line:       line,
		Column:     col,
		Suggestion: suggestion,
	}
}
