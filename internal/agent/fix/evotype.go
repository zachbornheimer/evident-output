// Package fix holds one go/analysis analyzer per API name the 1.1 vocabulary
// freeze retired, plus the registry that assembles them into the
// `evident-output fix` CLI subcommand. Detection is typed: every analyzer
// resolves a call's receiver or callee through pass.TypesInfo back to a
// declared type or function in the evo package, never by matching an
// identifier's spelling against a fixed word list. That is what lets
// out.Task("x").Blockf(...) and a *evo.TaskHandle stored under any local
// name both get caught, while a same-named method on an unrelated type
// never does.
package fix

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// EvoPackagePath is the import path every analyzer resolves receivers and
// callees against.
const EvoPackagePath = "github.com/zachbornheimer/evident-output"

// recvNamedType returns the evo package's named type behind a selector
// expression's receiver (TaskHandle, Output, ...), unwrapping one pointer
// level, or ("", false) when the receiver's type does not resolve to the
// evo package — including when the file being analyzed does not type-check
// cleanly enough for TypesInfo to have recorded a type.
func recvNamedType(info *types.Info, x ast.Expr) (string, bool) {
	t := info.TypeOf(x)
	if t == nil {
		return "", false
	}
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	} else if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return "", false
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Pkg().Path() != EvoPackagePath {
		return "", false
	}
	return obj.Name(), true
}

// isEvoPackageSelector reports whether sel.X is the local import alias for
// the evo package, resolved through the file's own import declarations
// rather than a fixed "evo" identifier check. Shared by every analyzer
// that needs to recognize a package-level evo selector (evo.Warn and
// evo.Evidence, both removed in 1.1, ...) even when sel.Sel itself no
// longer resolves to a member of the package (e.g. a removed function):
// sel.X still resolves as a valid import use.
func isEvoPackageSelector(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pkgName, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
	return ok && pkgName.Imported().Path() == EvoPackagePath
}

// callSelector splits a call expression into its selector, or (nil, nil)
// when the call is not a method/selector call (a bare func literal call,
// a parenthesized func value, etc — never anything this package fixes).
func callSelector(call *ast.CallExpr) *ast.SelectorExpr {
	sel, _ := call.Fun.(*ast.SelectorExpr)
	return sel
}

// isSelectorCalled reports whether the SelectorExpr on top of stack is
// itself the Fun of its immediately enclosing CallExpr — the shape every
// call-based analyzer's Preorder([]ast.Node{(*ast.CallExpr)(nil)}) walk
// already sees and handles. When it is not, the selector is a stand-alone
// reference used as a value: a method value (f := t.Warn) or a method
// expression ((*evo.TaskHandle).Kept), both of which produce a func value
// with no CallExpr wrapping the removed name at the reference site at
// all, so a call-based walk never finds them. defer always wraps a call
// (defer t.Step(1, 3, "x")), so it is never this shape — it is a plain
// call site the call-based walk above already handles.
func isSelectorCalled(stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	sel := stack[len(stack)-1]
	call, ok := stack[len(stack)-2].(*ast.CallExpr)
	return ok && call.Fun == sel
}

// isMethodExprRecv reports whether x — a selector's receiver expression —
// is itself a type, as in the method expression (*evo.TaskHandle).Kept,
// rather than a value, as in the method value t.Kept. A method
// expression's receiver can look exactly like an ordinary parenthesized
// value expression in the AST (ParenExpr around a StarExpr around a
// SelectorExpr); only go/types' own classification of the expression
// (TypeAndValue.IsType) tells the two apart.
func isMethodExprRecv(info *types.Info, x ast.Expr) bool {
	tv, ok := info.Types[x]
	return ok && tv.IsType()
}

// isNamedCompatTestShim reports whether stack's innermost enclosing
// function declaration is exactly <removedName>ForTest — the one-line
// export_test.go pattern (StepForTest, ...) that re-exposes a name the
// 1.1 freeze retired from the public surface so package-external tests
// can still call it during the compatibility window. Unlike an ordinary
// call site, rewriting the call inside its own eponymous shim does not
// migrate a caller off the retired name — it deletes the shim's only
// reason to exist and, for names whose replacement has different
// semantics (Step's atomic Progress+Phase update vs. Progress().Doing()'s
// two separate calls — see TestAPISugar_StepConcurrentWorkersNeverInterleave),
// silently regresses the very behavior the shim exists to keep testable.
func isNamedCompatTestShim(stack []ast.Node, removedName string) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		fn, ok := stack[i].(*ast.FuncDecl)
		if !ok {
			continue
		}
		return fn.Name.Name == removedName+"ForTest"
	}
	return false
}

// stripParens removes one layer of enclosing "(" ")" from a method
// expression receiver's source text ("(*evo.TaskHandle)" -> "*evo.TaskHandle"),
// so it can be reused verbatim as a func literal parameter's type.
func stripParens(s string) string {
	if len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		return s[1 : len(s)-1]
	}
	return s
}

// diag is a lightweight constructor for analysis.Diagnostic so every
// analyzer's Run body reads as one line per finding.
func diag(category string, node ast.Node, message string, fixes ...analysis.SuggestedFix) analysis.Diagnostic {
	return analysis.Diagnostic{
		Pos:            node.Pos(),
		End:            node.End(),
		Category:       category,
		Message:        message,
		SuggestedFixes: fixes,
	}
}

// exprEnclosingFunc finds the nearest enclosing func literal on an
// inspector stack, so an analyzer can tell whether a return statement is
// inside a Define callback (a func(context.Context) error literal) versus
// top-level code, where "return err" would not compile.
func exprEnclosingFunc(stack []ast.Node) *ast.FuncLit {
	for i := len(stack) - 1; i >= 0; i-- {
		if fn, ok := stack[i].(*ast.FuncLit); ok {
			return fn
		}
	}
	return nil
}
