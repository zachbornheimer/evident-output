package fix

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// StepAnalyzer is API-090: Progress wins over Step. Step(completed, total,
// name) is legacy; the 1.1 shape splits it into
// Progress(completed, total).Doing(name).
var StepAnalyzer = &analysis.Analyzer{
	Name:     "evostep",
	Doc:      "flags and fixes evo TaskHandle.Step, legacy syntax for Progress().Doing()",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runStep,
}

func runStep(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil || sel.Sel.Name != "Step" {
			return
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); !ok || recv != "TaskHandle" {
			return
		}
		if len(call.Args) != 3 {
			pass.Report(diag("API-090", call,
				"evo.TaskHandle.Step was removed in 1.1: Progress wins over Step — not rewritten: expected 3 arguments (completed, total, name)"))
			return
		}
		pass.Report(diag("API-090", call,
			"evo.TaskHandle.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal (task.Progress(i, total).Doing(name))",
			stepFix(pass, call, sel)))
	})
	reportStepValues(pass, insp)
	return nil, nil
}

// reportStepValues is API-090's method-value/method-expression case: a
// stand-alone reference to the removed Step — f := t.Step, defer t.Step,
// or the method expression (*evo.TaskHandle).Step — is a func value with
// no CallExpr wrapping the removed name, so runStep's call-based walk
// above never sees it. Step always took exactly (completed, total int,
// name string), so the rewrite is mechanical the same way stepFix's
// call-site rewrite is.
func reportStepValues(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel := n.(*ast.SelectorExpr)
		if sel.Sel.Name != "Step" || isSelectorCalled(stack) {
			return true
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); !ok || recv != "TaskHandle" {
			return true
		}
		pass.Report(diag("API-090", sel,
			"evo.TaskHandle.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal (task.Progress(i, total).Doing(name))",
			stepValueFix(pass, sel)))
		return true
	})
}

// stepValueFix wraps the removed Step method value/expression in a func
// literal with Step's own call shape (completed, total int, name string)
// that calls Progress(completed, total).Doing(name) inside.
func stepValueFix(pass *analysis.Pass, sel *ast.SelectorExpr) analysis.SuggestedFix {
	alias := evoAlias(pass, sel)
	var newText string
	if isMethodExprRecv(pass.TypesInfo, sel.X) {
		recvType := stripParens(text(pass, sel.X))
		newText = "func(recv " + recvType + ", completed, total int, name string) *" + alias + ".TaskHandle {\n\treturn recv.Progress(completed, total).Doing(name)\n}"
	} else {
		recv := text(pass, sel.X)
		newText = "func(completed, total int, name string) *" + alias + ".TaskHandle {\n\treturn " + recv + ".Progress(completed, total).Doing(name)\n}"
	}
	edits := []analysis.TextEdit{{Pos: sel.Pos(), End: sel.End(), NewText: []byte(newText)}}
	if imp := addEvoImport(pass, sel.Pos()); imp.NewText != nil {
		edits = append(edits, imp)
	}
	return analysis.SuggestedFix{
		Message:   "replace the Step method value/expression with a func literal calling Progress(completed, total).Doing(name)",
		TextEdits: edits,
	}
}

func stepFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr) analysis.SuggestedFix {
	recv := text(pass, sel.X)
	completed, total, name := text(pass, call.Args[0]), text(pass, call.Args[1]), text(pass, call.Args[2])
	newText := recv + ".Progress(" + completed + ", " + total + ").Doing(" + name + ")"
	return analysis.SuggestedFix{
		Message: "replace Step(completed, total, name) with Progress(completed, total).Doing(name)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)},
		},
	}
}
