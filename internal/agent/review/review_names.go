// Package review — EVO-STAMP-004 and EVO-FACT-002: Task names that are a
// fake phase or a bare noun, and a file path used as a Task just to show
// a problem.
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

var fakePhaseTaskNames = map[string]bool{
	"resolve": true, "finalize": true, "process": true, "handle": true,
}

var nounOnlyTaskNames = map[string]bool{
	"file integrity": true,
}

var pathFieldNames = map[string]bool{
	"File": true, "Path": true, "Filename": true, "FilePath": true, "Filepath": true,
}

func detectNounOnlyOrFakePhaseTaskName(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		name, pos, ok := taskNameLiteral(n, disproven)
		if !ok {
			return true
		}
		key := strings.ToLower(strings.TrimSpace(name))
		if !fakePhaseTaskNames[key] && !nounOnlyTaskNames[key] {
			return true
		}
		p := fset.Position(pos)
		findings = append(findings, stampFinding(
			"EVO-STAMP-004", "warning",
			"Task("+quoteLabel(name)+") is a noun-only or fake-phase name; Task names are verb+object",
			`rename Task(`+quoteLabel(name)+`) to Task("check file integrity") — a verb+object name for the actual work`,
			filename, p,
		))
		return true
	})
	return findings
}

func detectFilePathUsedAsProblemTask(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		findings = append(findings, fact002InBody(filename, body, fset, disproven)...)
	})
	return findings
}

func fact002InBody(filename string, body *ast.BlockStmt, fset *token.FileSet, disproven map[string]bool) []Finding {
	pathHandles := map[string]string{}
	var findings []Finding
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for i, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok || !isTaskCall(call) || !taskArgIsPathSelector(call) {
					continue
				}
				if i < len(assign.Lhs) {
					if id, ok := assign.Lhs[i].(*ast.Ident); ok {
						pathHandles[id.Name] = pathSelectorText(call.Args[0])
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
		case "Fail", "Block":
		default:
			return true
		}
		if !isLikelyEvoTaskReceiver(sel.X, disproven) {
			return true
		}
		pathExpr := ""
		switch recv := sel.X.(type) {
		case *ast.CallExpr:
			if isTaskCall(recv) && taskArgIsPathSelector(recv) {
				pathExpr = pathSelectorText(recv.Args[0])
			}
		case *ast.Ident:
			pathExpr = pathHandles[recv.Name]
		}
		if pathExpr == "" {
			return true
		}
		pos := fset.Position(call.Pos())
		recv := exprDottedNameOrDefault(sel.X, "owning")
		if isTaskCall(sel.X) {
			recv = "owning"
		}
		findings = append(findings, stampFinding(
			"EVO-FACT-002", "warning",
			"a file path is used as a Task name just to show a problem; that belongs on the owning Task as Fail/Fact",
			"call "+recv+`.Fail("checksum mismatch") and `+recv+`.Fact("file", `+pathExpr+`) on the Task that owns the work; do not declare Task(`+pathExpr+`)`,
			filename, pos,
		))
		return true
	})
	return findings
}

func taskArgIsPathSelector(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	return isPathSelector(call.Args[0])
}

func isPathSelector(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return pathFieldNames[sel.Sel.Name]
}

func pathSelectorText(e ast.Expr) string {
	if name := exprDottedName(e); name != "" {
		return name
	}
	return "issue.File"
}
