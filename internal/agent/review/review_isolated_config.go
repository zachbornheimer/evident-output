package review

import "go/ast"

// isolatedConfigs names the variables holding an evo.Config that may set
// Isolated, so evo.Init(cfg) is recognized as well as evo.Init(evo.Config{...}).
type isolatedConfigs map[string]bool

// bindAll applies bind to every name = value binding n introduces: an
// assignment (:= or =) or a var declaration. Callers visit bindings in
// source order, so a Config is known before the Init that uses it.
func (c isolatedConfigs) bindAll(n ast.Node, pkg string, outputs map[string]bool) {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		if len(stmt.Lhs) != len(stmt.Rhs) {
			return
		}
		for i, lhs := range stmt.Lhs {
			if id, ok := lhs.(*ast.Ident); ok {
				c.bind(id.Name, stmt.Rhs[i], pkg, outputs)
			}
		}
	case *ast.GenDecl:
		for _, spec := range stmt.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				c.bind(name.Name, vs.Values[i], pkg, outputs)
			}
		}
	}
}

// bind records name = value: a Config that may set Isolated makes name an
// isolated Config; an Init of one makes name a possibly Isolated Output.
func (c isolatedConfigs) bind(name string, value ast.Expr, pkg string, outputs map[string]bool) {
	if configMayIsolate(value, pkg) {
		c[name] = true
		return
	}
	call, ok := value.(*ast.CallExpr)
	if !ok || calledFuncDotted(call) != pkg+".Init" || len(call.Args) == 0 {
		return
	}
	if configMayIsolate(call.Args[0], pkg) || c[identName(call.Args[0])] {
		outputs[name] = true
	}
}

// configMayIsolate reports whether e is an evo.Config literal whose
// Isolated field is set to anything but the literal false — a variable
// counts, because some run of the program takes the Isolated branch.
func configMayIsolate(e ast.Expr, pkg string) bool {
	lit, ok := e.(*ast.CompositeLit)
	if !ok || !isConfigType(lit.Type, pkg) {
		return false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if ok && identName(kv.Key) == "Isolated" && identName(kv.Value) != "false" {
			return true
		}
	}
	return false
}

// isConfigType reports whether t spells pkg.Config.
func isConfigType(t ast.Expr, pkg string) bool {
	sel, ok := t.(*ast.SelectorExpr)
	return ok && isEvoIdent(sel.X, pkg) && sel.Sel.Name == "Config"
}
