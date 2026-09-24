package review

import (
	"go/ast"
	"strings"
)

// TaskHandle.Done was removed in 1.1 (ZYS-812): success resolves through
// Define, and the text Done(summary) carried is Summary result metadata
// (ZYS-971). API-032 rewrites each call site mechanically:
//
//	task.Done()            -> task.Define(func(ctx context.Context) error { ... })
//	task.Done("3 checked") -> task.Summary("3 checked").Define(...)
//	task.Done("%d", n)     -> task.Summary(fmt.Sprintf("%d", n)).Define(...)
//
// Inside task's own Define callback the Task is already defined, so the
// rewrite is Summary alone (or deleting a bare Done()).
//
// A zero-arg Done is also context.Context.Done and sync.WaitGroup.Done, so
// that shape is reported only on a receiver this file binds to a Task.

// defineBody is one `recv.Define(func ...)` callback literal's span.
type defineBody struct {
	recv string
	span srcSpan
}

// removedDoneScope is what inspectRemovedDone needs from the whole file:
// which expressions hold a Task, and where each Task's Define body sits.
type removedDoneScope struct {
	tasks        taskBindings
	defineBodies []defineBody
}

// newRemovedDoneScope collects f's Task bindings and every
// `recv.Define(func ...)` callback literal in it.
func newRemovedDoneScope(f *ast.File, d *recSurfaceDetector) removedDoneScope {
	scope := removedDoneScope{tasks: newTaskBindings(f, d.pkg)}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" || len(call.Args) != 1 {
			return true
		}
		if lit, ok := call.Args[0].(*ast.FuncLit); ok {
			scope.defineBodies = append(scope.defineBodies, defineBody{recv: exprDottedName(sel.X), span: d.nodeSpan(lit)})
		}
		return true
	})
	return scope
}

// insideOwnDefine reports whether offset sits in recv's own Define body.
func (s removedDoneScope) insideOwnDefine(recv string, offset int) bool {
	for _, b := range s.defineBodies {
		if b.recv == recv && b.span.contains(offset) {
			return true
		}
	}
	return false
}

// inspectRemovedDone reports one removed TaskHandle.Done call site with
// its Define/Summary rewrite. It reports nothing for a call it cannot
// prove is TaskHandle.Done.
func (d *recSurfaceDetector) inspectRemovedDone(call *ast.CallExpr, sel *ast.SelectorExpr) {
	if sel.Sel.Name != "Done" || d.doneScope == nil {
		return
	}
	certain := d.doneScope.tasks.IsTask(sel.X)
	if len(call.Args) == 0 && !certain {
		return
	}
	if len(call.Args) > 0 && !certain && (!isLikelyEvoReceiver(sel.X) || !isStringLit(call.Args[0])) {
		return
	}
	recv := d.nodeSrc(sel.X)
	summary := d.doneSummaryExpr(call.Args)
	old := d.nodeSrc(call)
	const msg = "TaskHandle.Done was removed in 1.1; success resolves through Define, and result text is Summary"
	if d.doneScope.insideOwnDefine(exprDottedName(sel.X), d.offset(call)) {
		if summary == "" {
			d.report(call, msg, "delete "+old+"; the Define callback returning nil already resolves "+recv)
		} else {
			d.report(call, msg, "replace "+old+" with "+recv+".Summary("+summary+")")
		}
		d.cover(call)
		return
	}
	next := recv
	if summary != "" {
		next += ".Summary(" + summary + ")"
	}
	next += ".Define(func(ctx context.Context) error { ... })"
	d.report(call, msg, "replace "+old+" with "+next)
	d.cover(call)
}

// doneSummaryExpr renders Done's removed printf-variadic summary as the
// single string Summary takes: "" for none, the literal for one argument,
// fmt.Sprintf(...) for a format with arguments.
func (d *recSurfaceDetector) doneSummaryExpr(args []ast.Expr) string {
	switch len(args) {
	case 0:
		return ""
	case 1:
		return d.nodeSrc(args[0])
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = d.nodeSrc(a)
	}
	return "fmt.Sprintf(" + strings.Join(parts, ", ") + ")"
}
