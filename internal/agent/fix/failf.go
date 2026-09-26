package fix

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// FailfAnalyzer is API-140: TaskHandle.Blockf, TaskHandle.Failf,
// Output.Failf, and the *Failure type they returned (Error/Next/
// NextCommand/Unwrap) were removed in 1.1 with no alias (owner vocabulary
// freeze, 2026-09-25: Fail/Block win as the one statement-form spelling
// in this family). Unlike Warn/Step/Kept/ReasonOption (also removed in
// 1.1), none of these get
// a SuggestedFix: folding a *f call's format string and args into a
// plain summary — and, for a %w verb specifically, deciding whether the
// wrapped error belongs in the summary text or a separate detail — is a
// semantic judgment call, not a mechanical rename (see registry.go's
// doc comment on why no fixer previously existed for this family). This
// analyzer exists purely to give consumer code a review finding and a
// `evident-output fix` diagnostic naming the removal, so a call site does
// not silently fail to compile with no MCP-visible signal.
var FailfAnalyzer = &analysis.Analyzer{
	Name:     "evofailf",
	Doc:      "flags evo Blockf/Failf/Failure calls and references removed in 1.1 (API-140), with no fix",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runFailf,
}

// failfMethodReceiver names the evo receiver type each removed *f method
// name is valid on. Output.Blockf never existed (Block is a Task-only
// verb), so "Blockf" only maps to TaskHandle here.
var failfMethodReceiver = map[string]string{
	"Failf":  "TaskHandle",
	"Blockf": "TaskHandle",
}

func runFailf(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil {
			return
		}
		reportFailfCall(pass, call, sel)
	})
	reportFailfValues(pass, insp)
	reportFailureType(pass, insp)
	return nil, nil
}

// reportFailfCall flags a Failf/Blockf call site whose receiver resolves
// (via go/types, independent of whether the call itself still type-checks
// — see recvNamedType) to an evo type that once carried it.
func reportFailfCall(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr) {
	name := sel.Sel.Name
	if name != "Failf" {
		reportSpecificFailfCall(pass, call, sel, name)
		return
	}
	recv, ok := recvNamedType(pass.TypesInfo, sel.X)
	if !ok || (recv != "TaskHandle" && recv != "Output") {
		return
	}
	pass.Report(diag("API-140", call,
		"(*evo."+recv+").Failf was removed in 1.1: Fail is the one statement-form spelling — fold the wrapped error text into the summary string by hand, then return the error separately (Define) or propagate it (fmt.Errorf/errors.Join) — not rewritten: no mechanical fix exists for a format string plus args"))
}

func reportSpecificFailfCall(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, name string) {
	if name != "Blockf" {
		return
	}
	recv, ok := recvNamedType(pass.TypesInfo, sel.X)
	if !ok || recv != "TaskHandle" {
		return
	}
	pass.Report(diag("API-140", call,
		"(*evo.TaskHandle).Blockf was removed in 1.1: Block is the one statement-form spelling — fold the wrapped error text into the summary string by hand, then return the error separately (Define) or propagate it (fmt.Errorf/errors.Join) — not rewritten: no mechanical fix exists for a format string plus args"))
}

// reportFailfValues is API-140's method-value/method-expression case: a
// stand-alone reference to the removed Failf/Blockf — f := t.Failf, or
// the method expression (*evo.TaskHandle).Failf — is a func value with no
// CallExpr wrapping the removed name, so reportFailfCall's call-based walk
// never sees it. (defer always wraps a call, so that shape is a plain
// call site the call-based walk above already finds — not this case.)
func reportFailfValues(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel := n.(*ast.SelectorExpr)
		if _, isFailf := failfMethodReceiver[sel.Sel.Name]; !isFailf || isSelectorCalled(stack) {
			return true
		}
		recv, ok := recvNamedType(pass.TypesInfo, sel.X)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Failf" && recv != "TaskHandle" && recv != "Output" {
			return true
		}
		if sel.Sel.Name == "Blockf" && recv != "TaskHandle" {
			return true
		}
		pass.Report(diag("API-140", sel,
			"(*evo."+recv+")."+sel.Sel.Name+" was removed in 1.1: Fail/Block is the one statement-form spelling — resolve this func value's call sites by hand, folding each wrapped error into its own summary string"))
		return true
	})
}

// reportFailureType flags evo.Failure used as a type — a var declaration,
// a parameter, a type assertion — the removed method-chain result type
// Failf/Blockf once returned. Detection is the same package-selector
// pattern isEvoPackageSelector already uses for other names removed in
// 1.1 (evo.Warn, capture-meaning evo.Evidence, both also removed in 1.1):
// sel.X still resolves to the evo package import even though sel.Sel
// itself (Failure) is no longer a member of it.
func reportFailureType(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.WithStack([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		sel := n.(*ast.SelectorExpr)
		if sel.Sel.Name != "Failure" || !isEvoPackageSelector(pass, sel) {
			return true
		}
		// A selector call (evo.Failure(...)) never existed — Failure
		// (removed in 1.1) was a struct type, never a constructor func —
		// but guard it anyway so a same-named future export is not
		// misreported as this type.
		if isSelectorCalled(stack) {
			return true
		}
		pass.Report(diag("API-140", sel,
			"evo.Failure was removed in 1.1 along with Failf/Blockf: there is no replacement type — a caller that inspected Failure.Error/Next/NextCommand/Unwrap now builds and returns a plain error, attaching remedies via ProblemOption (evo.Next/evo.NextCommand equivalents documented in docs/migration/1.1.md) before Fail/Block"))
		return true
	})
}
