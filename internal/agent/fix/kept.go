package fix

import (
	"go/ast"
	"go/types"

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
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil || sel.Sel.Name != "Kept" {
			return true
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); !ok || recv != "TaskHandle" {
			return true
		}
		// Kept was removed from the public API in 1.1; this fixer only
		// exists to migrate call sites in consumer code that still
		// reference the retired method. evo's own test files are excluded
		// on the same convention every other legacy-API fixer in this
		// package uses (isEvoOwnTestFile), so a fixture pinning the
		// removed method's own historical contract is not rewritten out
		// from under itself.
		if isEvoOwnTestFile(pass, call.Pos()) {
			return true
		}
		if len(call.Args) != 1 {
			pass.Report(diag("API-091", call,
				"evo.TaskHandle.Kept was removed in 1.1: Fact(\"kept\", reason.Name()) wins — not rewritten: expected exactly one Reason argument"))
			return true
		}
		msg := "evo.TaskHandle.Kept is not canonical vocabulary: a kept item is domain information, recorded with Fact(\"kept\", reason.Name()), not a third resolution alongside Succeeded/Skipped"
		// The old Kept resolved the Task (it called finish(Done)); Fact
		// never resolves anything. Auto-rewriting Kept(reason) to
		// Fact("kept", reason.Name()) is only safe inside a Define
		// callback, where returning nil is what resolves the Task —
		// Fact simply attaches the "kept" information alongside that
		// resolution. Outside a Define, the same rewrite would silently
		// leave the Task unresolved, so this reports without a fix and
		// tells the caller to add a resolving Define instead.
		if !isInsideDefineCallback(pass.TypesInfo, sel.X, stack) {
			pass.Report(diag("API-091", call,
				msg+" — not rewritten: this Kept call is outside a Define callback on the same receiver, and Fact never resolves a Task the way Kept used to; wrap it in a Define on this same receiver whose callback returns nil (or otherwise resolve the Task) before switching to Fact"))
			return true
		}
		recv := text(pass, sel.X)
		reason := text(pass, call.Args[0])
		newText := recv + `.Fact("kept", ` + reason + ".Name())"
		pass.Report(diag("API-091", call, msg,
			analysis.SuggestedFix{
				Message:   `replace Kept(reason) with Fact("kept", reason.Name())`,
				TextEdits: []analysis.TextEdit{{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)}},
			}))
		return true
	})
	reportKeptValues(pass, insp)
	return nil, nil
}

// isInsideDefineCallback reports whether stack's innermost enclosing
// function literal is passed directly as an argument to a `.Define(...)`
// call made on the SAME receiver as keptRecv (the Kept call's own
// receiver) — the only place a Kept rewrite to Fact is safe, because
// Define's own nil return is what resolves the Task; Fact never does.
// Receiver identity is checked via go/types object identity, not text: a
// per-item Kept inside a shared parent's Define (e.g.
// parent.Define(func(ctx){ for _, it := range items {
// g.Task(it).Kept(...) } })) or a Kept on an unrelated receiver inside
// someone else's Define (parent.Define(func(ctx){ other.Kept(r) })) both
// leave keptRecv's own Task unresolved by that Define, and must not be
// auto-rewritten.
func isInsideDefineCallback(info *types.Info, keptRecv ast.Expr, stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		lit, ok := stack[i].(*ast.FuncLit)
		if !ok {
			continue
		}
		if i == 0 {
			return false
		}
		call, ok := stack[i-1].(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" {
			return false
		}
		isArg := false
		for _, arg := range call.Args {
			if arg == lit {
				isArg = true
				break
			}
		}
		if !isArg {
			return false
		}
		if keptRecv == nil {
			// Method-value/method-expression case: the receiver is bound at
			// the later call site (keptFn(t, ...)), not here, so there is
			// nothing to compare identity against — the structural check
			// above (func literal passed directly to Define) is all that's
			// available.
			return true
		}
		return sameReceiver(info, keptRecv, sel.X)
	}
	return false
}

// sameReceiver reports whether a and b resolve to the identical
// types.Object — the only sound way to tell a Kept call's receiver and a
// Define call's receiver are the same live handle, since two different
// variables can share an identical textual spelling (e.g. two loop-scoped
// `task` locals) and one identifier can be reused across scopes.
func sameReceiver(info *types.Info, a, b ast.Expr) bool {
	aObj := receiverObject(info, a)
	bObj := receiverObject(info, b)
	return aObj != nil && aObj == bObj
}

// receiverObject resolves e to the types.Object it names, when e is a bare
// (optionally parenthesized) identifier — a variable, parameter, or field
// referenced directly by name. Any other shape (a call result, an index or
// selector chain, ...) has no single identity to compare and returns nil,
// which sameReceiver treats as "not provably the same receiver".
func receiverObject(info *types.Info, e ast.Expr) types.Object {
	switch x := e.(type) {
	case *ast.Ident:
		return info.ObjectOf(x)
	case *ast.ParenExpr:
		return receiverObject(info, x.X)
	default:
		return nil
	}
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
		msg := "evo.TaskHandle.Kept is not canonical vocabulary: a kept item is domain information, recorded with Fact(\"kept\", reason.Name())"
		// Same resolution concern as runKept's call-site case: a method
		// value/expression captured outside a Define callback would, once
		// rewritten to call Fact, never resolve the Task the way Kept
		// used to.
		if !isInsideDefineCallback(pass.TypesInfo, nil, stack) {
			pass.Report(diag("API-091", sel,
				msg+" — not rewritten: this Kept reference is outside a Define callback on the same receiver, and Fact never resolves a Task the way Kept used to; wrap the call site in a Define on this same receiver whose callback returns nil before switching to Fact"))
			return true
		}
		pass.Report(diag("API-091", sel, msg, keptValueFix(pass, sel)))
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
