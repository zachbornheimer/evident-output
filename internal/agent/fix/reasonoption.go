package fix

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// reasonOptionNames is ForSkip, OnTask and ReasonOption: the 1.1 freeze
// removed every ReasonOption because evo.Reason(name) takes only its name
// (E-122, ZYS-1180) — see the ReasonOptionAnalyzer doc comment.
var reasonOptionNames = map[string]bool{"ForSkip": true, "OnTask": true, "ReasonOption": true}

// ReasonOptionAnalyzer is API-120: ReasonOption, ForSkip and OnTask were
// removed in 1.1 — a Reason has no usage constraints. A call argument
// ForSkip()/OnTask() passed to evo.Reason(name, opts...) is deleted
// outright; any other appearance (a ReasonOption-typed parameter, a bare
// reference) is reported with no fix, since it isn't a call-argument
// deletion.
var ReasonOptionAnalyzer = &analysis.Analyzer{
	Name:     "evoreasonoption",
	Doc:      "flags and fixes evo ForSkip/OnTask/ReasonOption, removed in 1.1 (API-120)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runReasonOption,
}

func runReasonOption(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Every ForSkip()/OnTask() call that is a direct argument of an
	// evo.Reason(...) call: deleted along with its separating comma.
	handledArg := map[ast.Expr]bool{}
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Reason" || !isEvoPackageSelector(pass, sel) {
			return
		}
		for i, arg := range call.Args {
			if i == 0 {
				continue // the reason name itself
			}
			argSel, ok := argSelector(arg)
			if !ok || !isEvoPackageSelector(pass, argSel) || !reasonOptionNames[argSel.Sel.Name] {
				continue
			}
			handledArg[arg] = true
			pass.Report(diag("API-120", arg,
				"evo."+argSel.Sel.Name+" was removed in 1.1: a Reason has no usage constraints",
				deleteReasonArgFix(pass, call, i, argSel.Sel.Name)))
		}
	})

	// Any remaining reference (not a deleted Reason argument): a
	// ReasonOption-typed parameter, a stored value, a call used another
	// way. Reported with no fix.
	insp.Preorder([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node) {
		sel := n.(*ast.SelectorExpr)
		if !reasonOptionNames[sel.Sel.Name] || !isEvoPackageSelector(pass, sel) {
			return
		}
		if call, ok := selCallParent(insp, sel); ok && handledArg[call] {
			return
		}
		pass.Report(diag("API-120", sel,
			"evo."+sel.Sel.Name+" was removed in 1.1: a Reason has no usage constraints — not rewritten: not a direct evo.Reason(...) argument"))
	})
	return nil, nil
}

// argSelector returns a call argument's own call selector, when the
// argument is itself a call (ForSkip() as an argument is *ast.CallExpr
// whose Fun is the selector we key reasonOptionNames against).
func argSelector(arg ast.Expr) (*ast.SelectorExpr, bool) {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return sel, ok
}

// selCallParent finds the CallExpr a ForSkip()/OnTask() selector belongs
// to, so the second inspector pass can skip an argument the first pass
// already reported and fixed.
func selCallParent(insp *inspector.Inspector, sel *ast.SelectorExpr) (*ast.CallExpr, bool) {
	var found *ast.CallExpr
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if s, ok := call.Fun.(*ast.SelectorExpr); ok && s == sel {
			found = call
		}
	})
	return found, found != nil
}

func deleteReasonArgFix(pass *analysis.Pass, reasonCall *ast.CallExpr, index int, removedName string) analysis.SuggestedFix {
	arg := reasonCall.Args[index]
	start := arg.Pos()
	end := arg.End()
	// Consume the preceding ", " so deleting the last option doesn't leave
	// a trailing comma: evo.Reason("dirty", evo.ForSkip()) -> evo.Reason("dirty").
	if index > 0 {
		start = reasonCall.Args[index-1].End()
	}
	return analysis.SuggestedFix{
		Message:   "delete the removed " + removedName + " option: evo.Reason(name) takes only its name",
		TextEdits: []analysis.TextEdit{{Pos: start, End: end}},
	}
}
