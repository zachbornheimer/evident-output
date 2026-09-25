// Package review — API-100/API-101: Kept and ForSkip are removed in 1.1
// with no compatibility alias (vocabulary freeze: Summary/Skipped/Kept are
// not three equivalent outcomes — Kept is domain information, never
// vocabulary). A per-candidate Task a policy excludes is Skipped with an
// evo.Reason; a count such as "kept 383" is a Fact or part of the Summary.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// detectKeptCall is API-100: recv.Kept(reason) on a certain evo Task was
// removed in 1.1 with no alias. The mechanical rewrite is Skipped for the
// resolution and a Fact (or Summary text) for the "kept N" count itself.
func detectKeptCall(filename string, file *ast.File, fset *token.FileSet) []Finding {
	tasks := newTaskBindings(file, evoImportName(file))
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Kept" || !tasks.IsTask(sel.X) {
			return true
		}
		recv := types.ExprString(sel.X)
		args := make([]string, len(call.Args))
		for i, a := range call.Args {
			args[i] = types.ExprString(a)
		}
		argList := strings.Join(args, ", ")
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:  "API-100",
			Message: recv + ".Kept was removed in 1.1 with no alias — a policy-excluded item is Skipped, and a \"kept N\" count is domain information",
			File:    filename,
			Line:    pos.Line,
			Column:  pos.Column,
			Suggestion: "replace " + recv + ".Kept(" + argList + ") with " + recv + ".Skipped(" + argList + "); route a \"kept N\" count through " +
				recv + ".Fact(\"kept\", strconv.Itoa(n)) or the Task's Summary, not a resolution verb",
		})
		return true
	})
	return findings
}

// detectForSkipUsage is API-101: evo.ForSkip() is removed in 1.1 with no
// alias — a Reason carries no verb constraint now that Kept is gone. The
// call is resolved against the file's actual evident-output import name
// (evoImportName), the same way API-100 resolves Task/evo idents, so an
// aliased import (eo.ForSkip()) is caught and an unrelated local ForSkip
// with no evident-output import is not.
func detectForSkipUsage(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ForSkip" || !isEvoIdent(sel.X, pkg) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-101",
			Message:    pkg + ".ForSkip was removed in 1.1 with no alias — Skipped is the only disposition a Reason ever names now",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: pkg + ".ForSkip was removed in 1.1; delete the option — a Reason needs no verb constraint",
		})
		return true
	})
	return findings
}
