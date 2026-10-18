package review

import "go/ast"

// driverSinkMethod is one method body of a TerminalDriver type declared in
// the file, with the name its receiver is bound to.
type driverSinkMethod struct {
	recv string
	body *ast.BlockStmt
}

// terminalDriverMethods lists the method bodies of every type the file
// makes a TerminalDriver — the type that declares WriteLive, the one method
// only a renderer sink has. Such a type is where Evo's frames reach the
// terminal, so its writes to the writer it holds are the renderer itself.
func terminalDriverMethods(f *ast.File) []driverSinkMethod {
	byType := map[string][]driverSinkMethod{}
	drivers := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
			continue
		}
		field := fn.Recv.List[0]
		typeName := receiverTypeName(field.Type)
		if typeName == "" {
			continue
		}
		if fn.Name.Name == "WriteLive" {
			drivers[typeName] = true
		}
		if len(field.Names) == 1 {
			byType[typeName] = append(byType[typeName], driverSinkMethod{recv: field.Names[0].Name, body: fn.Body})
		}
	}
	var methods []driverSinkMethod
	for typeName := range drivers {
		methods = append(methods, byType[typeName]...)
	}
	return methods
}

// receiverTypeName is T for a receiver of type T or *T.
func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// writesOwnDriverSink reports whether call, whose first argument is w,
// sits inside a TerminalDriver method and writes to a field of that
// method's own receiver.
func writesOwnDriverSink(call *ast.CallExpr, w ast.Expr, methods []driverSinkMethod) bool {
	sel, ok := w.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	owner, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, m := range methods {
		if owner.Name == m.recv && m.body.Pos() <= call.Pos() && call.End() <= m.body.End() {
			return true
		}
	}
	return false
}
