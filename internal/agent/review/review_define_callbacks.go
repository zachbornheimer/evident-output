// Package review — the two kinds of Define callback the composition rules
// tell apart (contract §31): a Task's work callback, which receives a
// context, and a Group/Sequence topology builder, which receives its
// handle and declares structure only.
package review

import (
	"go/ast"
	"strings"
)

// defineCallback is one function literal passed to Define (or as the work
// of evo.Compute).
type defineCallback struct {
	lit     *ast.FuncLit
	builder bool // receives a *GroupHandle/*SequenceHandle, not a context
}

// defineCallbacks lists every Define/Compute callback in file.
func defineCallbacks(file *ast.File, evoPkg string) []defineCallback {
	var out []defineCallback
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		lit := callbackLiteral(call, evoPkg)
		if lit == nil {
			return true
		}
		if cb, ok := classifyCallback(lit); ok {
			out = append(out, cb)
		}
		return true
	})
	return out
}

// callbackLiteral is the function literal a Define(fn) or
// evo.Compute(task, fn) call carries, or nil.
func callbackLiteral(call *ast.CallExpr, evoPkg string) *ast.FuncLit {
	var arg ast.Expr
	switch {
	case calledFuncDotted(call) == evoPkg+".Compute" && len(call.Args) == 2:
		arg = call.Args[1]
	case isDefineCall(call):
		arg = call.Args[0]
	}
	lit, _ := arg.(*ast.FuncLit)
	return lit
}

func isDefineCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Define" && len(call.Args) == 1
}

func classifyCallback(lit *ast.FuncLit) (defineCallback, bool) {
	if isBuilderSignature(lit.Type) {
		return defineCallback{lit: lit, builder: true}, true
	}
	if _, ok := defineCallbackContextParamName(lit.Type); ok {
		return defineCallback{lit: lit}, true
	}
	return defineCallback{}, false
}

// isBuilderSignature is func(*evo.GroupHandle) or func(*evo.SequenceHandle).
func isBuilderSignature(ft *ast.FuncType) bool {
	if ft.Params == nil || len(ft.Params.List) != 1 {
		return false
	}
	star, ok := ft.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	name := typeExprName(star.X)
	return strings.HasSuffix(name, "GroupHandle") || strings.HasSuffix(name, "SequenceHandle")
}

func typeExprName(e ast.Expr) string {
	if sel, ok := e.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	return identName(e)
}

// inspectOwnBody walks body without entering nested function literals, so
// a callback is judged only by what it does itself.
func inspectOwnBody(body *ast.BlockStmt, visit func(ast.Node)) {
	ast.Inspect(body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		visit(n)
		return true
	})
}
