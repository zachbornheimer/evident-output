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
	return nil, nil
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
