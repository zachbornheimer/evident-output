package fix

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// KeptAnalyzer is API-091: Kept is not canonical vocabulary. It never meant
// "this item did not run" (that is Skipped); it recorded a kept item as
// domain information, so Kept(reason) rewrites to Fact("kept",
// reason.Name()) — the owner vocabulary freeze's own migration (see
// CHANGELOG.md and API-062), not to Skipped, which would silently change
// the task's outcome from Done to Skipped.
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
		// Kept was removed from the public API in 1.1; this fixer only
		// exists to migrate call sites in consumer code that still
		// reference the retired method. evo's own test files are excluded
		// on the same convention every other legacy-API fixer in this
		// package uses (isEvoOwnTestFile), so a fixture pinning the
		// removed method's own historical contract is not rewritten out
		// from under itself.
		if isEvoOwnTestFile(pass, call.Pos()) {
			return
		}
		if len(call.Args) != 1 {
			pass.Report(diag("API-091", call,
				"evo.TaskHandle.Kept was removed in 1.1: Fact(\"kept\", reason.Name()) wins — not rewritten: expected exactly one Reason argument"))
			return
		}
		recv := text(pass, sel.X)
		reason := text(pass, call.Args[0])
		newText := recv + `.Fact("kept", ` + reason + ".Name())"
		pass.Report(diag("API-091", call,
			"evo.TaskHandle.Kept is not canonical vocabulary: a kept item is domain information, recorded with Fact(\"kept\", reason.Name()), not a third resolution alongside Succeeded/Skipped",
			analysis.SuggestedFix{
				Message:   `replace Kept(reason) with Fact("kept", reason.Name())`,
				TextEdits: []analysis.TextEdit{{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)}},
			}))
	})
	reportKeptValues(pass, insp)
	return nil, nil
}

// reportKeptValues is API-091's method-value/method-expression case: a
// stand-alone reference to the removed Kept — f := t.Kept, or the method
// expression (*evo.TaskHandle).Kept — is a func value with no CallExpr
// wrapping the removed name, so runKept's call-based walk above never
// sees it. (defer always wraps a call, e.g. defer t.Kept(reason), so
// that shape is a plain call site the call-based walk above already
// finds — not this case.) Kept always took exactly one Reason argument,
// so the rewrite is mechanical the same way runKept's call-site rewrite
// is.
func reportKeptValues(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel := n.(*ast.SelectorExpr)
		if sel.Sel.Name != "Kept" || isSelectorCalled(stack) {
			return true
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); !ok || recv != "TaskHandle" {
			return true
		}
		if isEvoOwnTestFile(pass, sel.Pos()) {
			return true
		}
		pass.Report(diag("API-091", sel,
			"evo.TaskHandle.Kept is not canonical vocabulary: a kept item is domain information, recorded with Fact(\"kept\", reason.Name())",
			keptValueFix(pass, sel)))
		return true
	})
}

// keptValueFix wraps the removed Kept method value/expression in a func
// literal with Kept's own call shape (one Reason argument) that calls
// Fact("kept", reason.Name()) inside.
//
// The plain-value branch captures the receiver once, matching a method
// value's own evaluate-once-at-creation semantics (see warnValueFix); the
// method-expression branch takes the receiver as an explicit parameter
// evaluated per call, which already matches (*evo.TaskHandle).Kept's own
// semantics.
func keptValueFix(pass *analysis.Pass, sel *ast.SelectorExpr) analysis.SuggestedFix {
	alias := evoAlias(pass, sel)
	var newText string
	if isMethodExprRecv(pass.TypesInfo, sel.X) {
		recvType := stripParens(text(pass, sel.X))
		newText = "func(recv " + recvType + ", reason " + alias + ".TaxonomyReason) {\n\trecv.Fact(\"kept\", reason.Name())\n}"
	} else {
		recv := text(pass, sel.X)
		newText = "func() func(reason " + alias + ".TaxonomyReason) {\n\trecv := " + recv + "\n\treturn func(reason " + alias + ".TaxonomyReason) {\n\t\trecv.Fact(\"kept\", reason.Name())\n\t}\n}()"
	}
	edits := []analysis.TextEdit{{Pos: sel.Pos(), End: sel.End(), NewText: []byte(newText)}}
	if imp := addEvoImport(pass, sel.Pos()); imp.NewText != nil {
		edits = append(edits, imp)
	}
	return analysis.SuggestedFix{
		Message:   `replace the Kept method value/expression with a func literal calling Fact("kept", reason.Name())`,
		TextEdits: edits,
	}
}
