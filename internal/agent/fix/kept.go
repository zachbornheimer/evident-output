package fix

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// KeptAnalyzer is API-091: Kept is not canonical vocabulary. A per-candidate
// Task intentionally not executed is Skipped; Kept(reason) rewrites
// directly to Skipped(reason) — the TaxonomyReason value itself is
// unchanged, only the outcome verb.
var KeptAnalyzer = &analysis.Analyzer{
	Name:     "evokept",
	Doc:      "flags and fixes evo TaskHandle.Kept, legacy syntax for Skipped",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runKept,
}

func runKept(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil || sel.Sel.Name != "Kept" {
			return
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); !ok || recv != "TaskHandle" {
			return
		}
		if len(call.Args) != 1 {
			pass.Report(diag("API-091", call,
				"evo.TaskHandle.Kept was removed in 1.1: Skipped wins — not rewritten: expected exactly one Reason argument"))
			return
		}
		recv := text(pass, sel.X)
		reason := text(pass, call.Args[0])
		newText := recv + ".Skipped(" + reason + ")"
		pass.Report(diag("API-091", call,
			"evo.TaskHandle.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped(reason)",
			analysis.SuggestedFix{
				Message:   "replace Kept(reason) with Skipped(reason)",
				TextEdits: []analysis.TextEdit{{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)}},
			}))
	})
	return nil, nil
}
