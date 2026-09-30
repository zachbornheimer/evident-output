package review

import "go/ast"

// outputBindings is which names in one file certainly hold an *evo.Output:
// fields and variables typed *evo.Output, and variables assigned from
// evo.Init / evo.Default. The review package loads no imports, so receiver
// types are proven from these bindings rather than from go/types.
type outputBindings struct {
	names map[string]bool
}

func newOutputBindings(file *ast.File, evoPkg string) outputBindings {
	b := outputBindings{names: map[string]bool{}}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			if isOutputType(n.Type, evoPkg) {
				b.bindAll(n.Names)
			}
		case *ast.ValueSpec:
			b.bindValueSpec(n, evoPkg)
		case *ast.AssignStmt:
			b.bindAssign(n, evoPkg)
		}
		return true
	})
	return b
}

func (b outputBindings) bindAll(names []*ast.Ident) {
	for _, name := range names {
		b.names[name.Name] = true
	}
}

func (b outputBindings) bindValueSpec(spec *ast.ValueSpec, evoPkg string) {
	for i, name := range spec.Names {
		typed := spec.Type != nil && isOutputType(spec.Type, evoPkg)
		if typed || i < len(spec.Values) && isOutputConstructor(spec.Values[i], evoPkg) {
			b.names[name.Name] = true
		}
	}
}

func (b outputBindings) bindAssign(assign *ast.AssignStmt, evoPkg string) {
	if len(assign.Lhs) != len(assign.Rhs) {
		return
	}
	for i, rhs := range assign.Rhs {
		if id, ok := assign.Lhs[i].(*ast.Ident); ok && isOutputConstructor(rhs, evoPkg) {
			b.names[id.Name] = true
		}
	}
}

// has reports whether expr is a bound identifier or a field bound by name.
func (b outputBindings) has(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return b.names[e.Name]
	case *ast.SelectorExpr:
		return b.names[e.Sel.Name]
	}
	return false
}

func isOutputType(t ast.Expr, evoPkg string) bool {
	star, ok := t.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "Output" && isEvoIdent(x.X, evoPkg)
	case *ast.Ident:
		return x.Name == "Output"
	}
	return false
}

func isOutputConstructor(e ast.Expr, evoPkg string) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && isEvoIdent(sel.X, evoPkg) && (sel.Sel.Name == "Init" || sel.Sel.Name == "Default")
}
