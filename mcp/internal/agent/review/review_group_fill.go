// Package review — API-039 counts the children a function declares on a
// Group. Another function can add more, so a Group the function hands on
// is never judged on its own Task calls alone.
package review

import "go/ast"

// groupFilledBeyondTasks reports whether body declares children on the
// Group variable name some way other than name.Task(...): a nested
// Group/Sequence, a Build callback, or passing the Group to another call
// (After(name) only orders work and adds no child).
func groupFilledBeyondTasks(body *ast.BlockStmt, name string) bool {
	filled := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || filled {
			return !filled
		}
		if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel {
			if identName(sel.X) == name && (handleKinds[sel.Sel.Name] && sel.Sel.Name != "Task" || sel.Sel.Name == "Build") {
				filled = true
				return false
			}
			if sel.Sel.Name == "After" {
				return true
			}
		}
		for _, arg := range call.Args {
			if identName(arg) == name {
				filled = true
			}
		}
		return !filled
	})
	return filled
}
