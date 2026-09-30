// Package review — API-068 (ZYS-1368): a Task's Define callback declares
// child Tasks, Groups or Sequences. A Task is never a container (contract
// §31); structure belongs to a Group or Sequence whose own Define builder
// declares the children.
package review

import (
	"go/ast"
	"go/token"
)

// detectChildrenDeclaredInTaskDefine is API-068.
func detectChildrenDeclaredInTaskDefine(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	for _, cb := range defineCallbacks(file, evoImportName(file)) {
		if cb.builder {
			continue
		}
		if child := firstChildDeclaration(cb.lit.Body); child != nil {
			pos := fset.Position(child.Pos())
			findings = append(findings, Finding{
				RuleID:     "API-068",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Message:    "child " + childKind(child) + " declared inside a Task's Define callback: a Task is never a container, so its children appear after the run starts and no row shows the work up front",
				Suggestion: "declare a Group (or Sequence) for the parent and let its Define builder declare the children: group.Define(func(g *evo.GroupHandle) { g.Task(name).Define(work) })",
			})
		}
	}
	return findings
}

// firstChildDeclaration is the first container.Task/Group/Sequence(name)
// call in the callback's own body.
func firstChildDeclaration(body *ast.BlockStmt) *ast.CallExpr {
	var first *ast.CallExpr
	inspectOwnBody(body, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok || first != nil || len(call.Args) != 1 {
			return
		}
		if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && handleKinds[sel.Sel.Name] {
			first = call
		}
	})
	return first
}

func childKind(call *ast.CallExpr) string {
	return call.Fun.(*ast.SelectorExpr).Sel.Name
}
