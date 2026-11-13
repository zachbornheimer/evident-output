package review

import (
	"go/ast"
	"go/token"
)

var taskWindowMethods = map[string]bool{
	"Doing": true, "Progress": true, "Writer": true, "Evidence": true,
	"Each": true, "EachN": true, "Run": true, "Bytes": true,
}

func detectInstantDone(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		findings = append(findings, instantDoneInFunc(filename, fn.Body, fset)...)
		return false
	})
	return findings
}

func instantDoneInFunc(filename string, body *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	windowed := map[string]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for i, rhs := range assign.Rhs {
				if !isTaskCall(rhs) {
					continue
				}
				if i < len(assign.Lhs) {
					if id, ok := assign.Lhs[i].(*ast.Ident); ok {
						windowed[id.Name] = false
					}
				}
			}
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "Done":
			if isTaskCall(sel.X) {
				pos := fset.Position(call.Pos())
				findings = append(findings, instantDoneFinding(filename, pos, chainedTaskName(sel.X)))
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok {
				if seen, ok := windowed[id.Name]; ok && !seen {
					pos := fset.Position(call.Pos())
					findings = append(findings, instantDoneFinding(filename, pos, id.Name))
				}
			}
		default:
			if taskWindowMethods[sel.Sel.Name] {
				if id, ok := sel.X.(*ast.Ident); ok {
					if _, ok := windowed[id.Name]; ok {
						windowed[id.Name] = true
					}
				}
			}
		}
		return true
	})
	return findings
}

func instantDoneFinding(filename string, pos token.Position, recv string) Finding {
	suggestion := "call Doing(...) before Done so the row spins while the work runs"
	if recv != "" {
		suggestion = recv + ".Doing(\"resolving\") before " + recv + ".Done(...)"
	}
	return Finding{
		RuleID:     "FP-005",
		Severity:   "warning",
		Message:    "Task is Done with no running window; the row first appears already complete",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: suggestion,
	}
}

func isTaskCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Task" || sel.Sel.Name == "Item"
}

func chainedTaskName(e ast.Expr) string {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return exprDottedName(sel.X) + ".Task"
}

func detectSingletonDisplayGroup(filename string, file *ast.File, fset *token.FileSet) []Finding {
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
		if sel.Sel.Name == "Task" && isDisplayGroupCall(sel.X) {
			pos := fset.Position(sel.Pos())
			findings = append(findings, singletonGroupFinding(filename, pos, "DisplayGroup"))
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
			if !isDisplayGroupCall(rhs) {
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
		if g.inLoop || g.tasks != 1 {
			continue
		}
		findings = append(findings, singletonGroupFindingAt(filename, g.line, g.col, name))
	}
	return findings
}

func isDisplayGroupCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "DisplayGroup"
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
	suggestion := "use a lone Task instead of a 1-child DisplayGroup"
	if recv != "" && recv != "DisplayGroup" {
		suggestion = "replace " + recv + " with out.Task(...) — a DisplayGroup of one child is a single line"
	}
	return Finding{
		RuleID:     "API-039",
		Severity:   "warning",
		Message:    "DisplayGroup has exactly one child; it renders as a 0/1 complete header over a single row",
		File:       filename,
		Line:       line,
		Column:     col,
		Suggestion: suggestion,
	}
}
