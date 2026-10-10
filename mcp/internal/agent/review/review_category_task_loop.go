// Package review — API-070 (ZYS-1368): one Task that loops over items and
// does context-bound work for each. Every item deserves its own outcome, so
// the category is a Group and each item is a Task under it. A loop that
// reports Progress or Doing is one long operation narrating itself, not a
// set of outcomes, and is left alone.
package review

import (
	"go/ast"
	"go/token"
)

// loopNarrationCalls are the methods a single long Task uses to report a
// loop's progress.
var loopNarrationCalls = map[string]bool{"Progress": true, "Doing": true, "Writer": true}

// detectCategoryTaskLoopingOverItems is API-070.
func detectCategoryTaskLoopingOverItems(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	for _, cb := range defineCallbacks(file, evoImportName(file)) {
		if cb.builder {
			continue
		}
		ctxName, _ := defineCallbackContextParamName(cb.lit.Type)
		if loop := itemLoopWithContextWork(cb.lit.Body, ctxName); loop != nil {
			pos := fset.Position(loop.Pos())
			findings = append(findings, Finding{
				RuleID:     "API-070",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Message:    "one Task loops over items and does work for each item: each item deserves its own outcome, and a category Task hides which item failed or was skipped",
				Suggestion: "declare a Group for the category and one Task per item: group := out.Group(category); for _, item := range items { group.Task(verb + \" \" + item).Define(work) }",
			})
		}
	}
	return findings
}

// itemLoopWithContextWork is the first range loop in the callback's own
// body whose iterations pass the scheduler context to work and never
// narrate progress. ctxName "" (an unnamed context) matches nothing.
func itemLoopWithContextWork(body *ast.BlockStmt, ctxName string) *ast.RangeStmt {
	if ctxName == "" {
		return nil
	}
	var found *ast.RangeStmt
	inspectOwnBody(body, func(n ast.Node) {
		loop, ok := n.(*ast.RangeStmt)
		if ok && found == nil && stmtLeafCallsWithIdent(loop.Body, ctxName) && !narratesLoop(loop.Body) {
			found = loop
		}
	})
	return found
}

func narratesLoop(body *ast.BlockStmt) bool {
	narrates := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && loopNarrationCalls[sel.Sel.Name] {
			narrates = true
		}
		return !narrates
	})
	return narrates
}
