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

// recvTypeUnresolved reports whether x's type failed to resolve at all —
// info.TypeOf returns nil, or an invalid type — which is the only case a
// name fully removed from evo (so go/types has no Uses entry) can produce.
// A receiver whose type DID resolve, just to something outside the evo
// package (a *slog.Logger, a local type with a same-named method), must
// never fall through to the untyped alias-tracing fallback: that fallback
// matches on identifier spelling alone and would rewrite an unrelated
// type's call into an evo one that does not compile.
func recvTypeUnresolved(info *types.Info, x ast.Expr) bool {
	t := info.TypeOf(x)
	return t == nil || t == types.Typ[types.Invalid]
}

// packageFunc returns the evo package-level function name a selector's
// Sel identifier resolves to — evo.Init, evo.Warn (removed in 1.1), and
// so on — or ("", false) when it resolves to anything else (a method, a
// local, another package).
func packageFunc(info *types.Info, sel *ast.SelectorExpr) (string, bool) {
	use, ok := info.Uses[sel.Sel]
	if !ok {
		return "", false
	}
	fn, ok := use.(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != EvoPackagePath {
		return "", false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil {
		return "", false
	}
	return fn.Name(), true
}

// callSelector splits a call expression into its selector, or (nil, nil)
// when the call is not a method/selector call (a bare func literal call,
// a parenthesized func value, etc — never anything this package fixes).
func callSelector(call *ast.CallExpr) *ast.SelectorExpr {
	sel, _ := call.Fun.(*ast.SelectorExpr)
	return sel
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

// aliasReceiver reports whether a call receiver expression plausibly holds
// an evo value, used only as a fallback when a name has already been
// fully removed from the evo package (so go/types has no Uses entry to
// resolve at all — the identifier is a compile error, not a type
// mismatch): the import alias itself, or a local whose declared type or
// most recent assignment chains back to it. This mirrors
// internal/agent/review's evoImportName/evoValuedIdents tracing — the
// established pattern in this codebase for names that may not type-check
// — rather than inventing a second heuristic.
func aliasReceiver(alias string, x ast.Expr, evoLocals map[string]bool) bool {
	switch e := x.(type) {
	case *ast.Ident:
		return e.Name == alias || evoLocals[e.Name]
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			return aliasReceiver(alias, sel.X, evoLocals)
		}
	case *ast.ParenExpr:
		return aliasReceiver(alias, e.X, evoLocals)
	}
	return false
}

// evoImportAlias returns the local name the enclosing file imports the
// evo package under, or "" when the file does not import it.
func evoImportAlias(pass *analysis.Pass, at ast.Node) string {
	f := enclosingFile(pass, at.Pos())
	if f == nil {
		return ""
	}
	for _, imp := range f.Imports {
		if importPath(imp) != EvoPackagePath {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				return ""
			}
			return imp.Name.Name
		}
		return "evo"
	}
	return ""
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
