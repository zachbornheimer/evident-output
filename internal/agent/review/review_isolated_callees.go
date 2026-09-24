package review

import "go/ast"

// reachedCall is one call a Run callback reaches: in the callback itself
// (via is empty) or inside the package function named via.
type reachedCall struct {
	call *ast.CallExpr
	src  goSource
	via  string
}

// reach visits every call run's callback makes, following calls to the
// package's own top-level functions transitively. Each function is
// entered once per run, so recursion terminates.
func (p *isolatedPackage) reach(run isolatedRun, visit func(reachedCall)) {
	entered := map[string]bool{}
	var walk func(body ast.Node, src goSource, via string)
	walk = func(body ast.Node, src goSource, via string) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			visit(reachedCall{call: call, src: src, via: via})
			if site, ok := p.callee(call.Fun, entered); ok {
				walk(site.decl.Body, site.src, site.decl.Name.Name)
			}
			return true
		})
	}
	switch cb := run.callback.(type) {
	case *ast.FuncLit:
		walk(cb.Body, run.src, "")
	case *ast.Ident:
		if site, ok := p.callee(cb, entered); ok {
			walk(site.decl.Body, site.src, site.decl.Name.Name)
		}
	}
}

// callee resolves fun to a package function not yet entered, and marks
// it entered.
func (p *isolatedPackage) callee(fun ast.Expr, entered map[string]bool) (funcSite, bool) {
	id, ok := fun.(*ast.Ident)
	if !ok || entered[id.Name] {
		return funcSite{}, false
	}
	site, ok := p.funcs[id.Name]
	if !ok {
		return funcSite{}, false
	}
	entered[id.Name] = true
	return site, true
}
