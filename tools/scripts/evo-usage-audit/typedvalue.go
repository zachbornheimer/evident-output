package main

import "go/ast"

// typedValueIndex is what makes cross-file "typed value" usage visible: a
// per-directory (Go package) index, built once before any declaration in
// that directory is classified, of which struct fields and named types
// reach evo through their declared type — regardless of which file in the
// package declares them. This is what lets a method in doctor.go resolve
// application.out (declared in app.go) as evo-typed even though doctor.go's
// own text never spells "evo.".
//
// GenDecl var/const initializers are intentionally excluded — this index
// only serves classifyTypedValue, which only classifies function bodies
// (see the work order's stated asymmetry, echoed in --help).
type typedValueIndex struct {
	// evoFields maps a struct type name to the set of its field names whose
	// declared type references evo.
	evoFields map[string]map[string]bool
	// evoNamedTypes marks a non-struct named type (type X = evo.Y, or
	// type X evo.Y) whose declared type references evo.
	evoNamedTypes map[string]bool
}

// buildTypedValueIndex scans every file already parsed for one directory
// and returns the struct-field and named-type evo index for that directory.
// It only looks at top-level type declarations — evo reached through a
// value's initializer, rather than its declared type, is out of scope here
// (classifyGenDecl already handles that for the declaration doing the
// initializing).
func buildTypedValueIndex(files []parsedGoFile) typedValueIndex {
	index := typedValueIndex{
		evoFields:     map[string]map[string]bool{},
		evoNamedTypes: map[string]bool{},
	}
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				indexTypeSpec(ts, pf.idents, index)
			}
		}
	}
	return index
}

// indexTypeSpec records one named type into index: a struct records each
// evo-typed field by name; any other named type records itself when its
// declared type references evo directly (a defined or aliased evo type).
func indexTypeSpec(ts *ast.TypeSpec, idents map[string]bool, index typedValueIndex) {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		if referencesEvoIdent(ts.Type, idents) {
			index.evoNamedTypes[ts.Name.Name] = true
		}
		return
	}
	if st.Fields == nil {
		return
	}
	for _, field := range st.Fields.List {
		if !referencesEvoIdent(field.Type, idents) {
			continue
		}
		for _, name := range field.Names {
			fields, ok := index.evoFields[ts.Name.Name]
			if !ok {
				fields = map[string]bool{}
				index.evoFields[ts.Name.Name] = fields
			}
			fields[name.Name] = true
		}
	}
}

// funcParamBindings maps a function's receiver and parameter names to their
// declared type name (pointer stripped) — the only bindings classifyTypedValue
// trusts. Local variables declared inside the body (var or :=) are
// deliberately left unbound: resolving them would require a real type
// checker, and per the work order's precision-over-recall stance, a base
// whose type can't be determined this way must never classify.
func funcParamBindings(fd *ast.FuncDecl) map[string]string {
	bindings := map[string]string{}
	addFieldList(fd.Recv, bindings)
	if fd.Type != nil {
		addFieldList(fd.Type.Params, bindings)
	}
	return bindings
}

func addFieldList(fl *ast.FieldList, bindings map[string]string) {
	if fl == nil {
		return
	}
	for _, field := range fl.List {
		typeName, ok := identTypeName(field.Type)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			bindings[name.Name] = typeName
		}
	}
}

// identTypeName reports the bare identifier name of expr's type, stripping
// one leading pointer star. Anything else (a selector into another package,
// a generic instantiation, an interface literal, ...) is left unresolved.
func identTypeName(expr ast.Expr) (string, bool) {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	id, ok := expr.(*ast.Ident)
	if !ok {
		return "", false
	}
	return id.Name, true
}

// classifyTypedValue reports whether fd's body reaches evo only through a
// typed value: a receiver or parameter whose declared type is itself an
// evo-typed named type, or one of whose fields (looked up by the owner's
// type name, never by bare field name alone) is evo-typed. Callers must
// only invoke this after direct and evo-typed-signature have both missed.
func classifyTypedValue(fd *ast.FuncDecl, index typedValueIndex) usage {
	if fd.Body == nil {
		return usageNone
	}
	bindings := funcParamBindings(fd)
	if len(bindings) == 0 {
		return usageNone
	}
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if isEvoTypedExpr(sel.X, bindings, index) {
			found = true
			return false
		}
		return true
	})
	if found {
		return usageEvoTypedValue
	}
	return usageNone
}

// isEvoTypedExpr reports whether expr is an evo-typed value under bindings:
// either a bound identifier whose own type is a recorded evo named type, or
// a field access (owner.field) whose owner's bound type has field recorded
// as evo-typed. An identifier absent from bindings — a local variable, a
// package-level name, anything this pass didn't resolve — never matches:
// the base's type couldn't be determined, so it's left unclassified.
func isEvoTypedExpr(expr ast.Expr, bindings map[string]string, index typedValueIndex) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		typeName, ok := bindings[e.Name]
		return ok && index.evoNamedTypes[typeName]
	case *ast.SelectorExpr:
		ownerIdent, ok := e.X.(*ast.Ident)
		if !ok {
			return false
		}
		ownerType, ok := bindings[ownerIdent.Name]
		if !ok {
			return false
		}
		fields := index.evoFields[ownerType]
		return fields != nil && fields[e.Sel.Name]
	default:
		return false
	}
}
