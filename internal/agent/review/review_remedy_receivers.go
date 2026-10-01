package review

import "go/ast"

// declaredReceiverTypes proves a receiver is NOT an evo TaskHandle, Output, or
// Failure: a name whose every declaration in the file spells a type that is
// known and is not an evo type (a local struct or interface, a builtin, an
// imported non-evo package type such as bytes.Buffer). The review package loads
// no imports, so this is spelled-type evidence, not go/types. A name with no
// declaration here, or with any declaration whose type is not spelled (a call
// result, a chain), stays unproven and its Next call is reported.
type declaredReceiverTypes struct {
	evoPkg string
	// evoAliases are local type names that spell an evo type (type T = evo.TaskHandle).
	evoAliases map[string]bool
	// nonEvo maps a value name to one flag per declaration: true when that
	// declaration's type is known and not evo.
	nonEvo map[string][]bool
}

func newDeclaredReceiverTypes(file *ast.File, evoPkg string) declaredReceiverTypes {
	d := declaredReceiverTypes{evoPkg: evoPkg, evoAliases: map[string]bool{}, nonEvo: map[string][]bool{}}
	ast.Inspect(file, func(n ast.Node) bool {
		if spec, ok := n.(*ast.TypeSpec); ok && d.spellsEvoType(spec.Type) {
			d.evoAliases[spec.Name.Name] = true
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			d.declare(n.Names, d.isKnownNonEvo(n.Type))
		case *ast.ValueSpec:
			d.declareValueSpec(n)
		case *ast.AssignStmt:
			d.declareDefined(n)
		}
		return true
	})
	return d
}

func (d declaredReceiverTypes) declare(names []*ast.Ident, known bool) {
	for _, name := range names {
		d.nonEvo[name.Name] = append(d.nonEvo[name.Name], known)
	}
}

func (d declaredReceiverTypes) declareValueSpec(spec *ast.ValueSpec) {
	known := spec.Type != nil && d.isKnownNonEvo(spec.Type)
	d.declare(spec.Names, known)
}

func (d declaredReceiverTypes) declareDefined(assign *ast.AssignStmt) {
	if assign.Tok.String() != ":=" {
		return
	}
	for i, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		known := len(assign.Lhs) == len(assign.Rhs) && d.isKnownNonEvoLiteral(assign.Rhs[i])
		d.declare([]*ast.Ident{id}, known)
	}
}

// proves reports whether x is a name declared only with known non-evo types.
func (d declaredReceiverTypes) proves(x ast.Expr) bool {
	var name string
	switch e := x.(type) {
	case *ast.Ident:
		name = e.Name
	case *ast.SelectorExpr:
		name = e.Sel.Name
	default:
		return false
	}
	flags := d.nonEvo[name]
	for _, known := range flags {
		if !known {
			return false
		}
	}
	return len(flags) > 0
}

// isKnownNonEvo reports whether the spelled type t is known and not an evo type.
func (d declaredReceiverTypes) isKnownNonEvo(t ast.Expr) bool {
	switch t := t.(type) {
	case *ast.StarExpr:
		return d.isKnownNonEvo(t.X)
	case *ast.Ident:
		return !isBareEvoTypeName(t.Name) && !d.evoAliases[t.Name]
	case *ast.SelectorExpr:
		return !isEvoIdent(t.X, d.evoPkg)
	case *ast.InterfaceType, *ast.StructType, *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType:
		return true
	}
	return false
}

// spellsEvoType reports whether t is spelled *evo.X or evo.X.
func (d declaredReceiverTypes) spellsEvoType(t ast.Expr) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	sel, ok := t.(*ast.SelectorExpr)
	return ok && isEvoIdent(sel.X, d.evoPkg)
}

// isKnownNonEvoLiteral is T{} or &T{} where T is a known non-evo type.
func (d declaredReceiverTypes) isKnownNonEvoLiteral(e ast.Expr) bool {
	if unary, ok := e.(*ast.UnaryExpr); ok {
		e = unary.X
	}
	lit, ok := e.(*ast.CompositeLit)
	return ok && lit.Type != nil && d.isKnownNonEvo(lit.Type)
}

// isBareEvoTypeName is an evo type spelled without a package qualifier (a dot
// import).
func isBareEvoTypeName(name string) bool {
	return name == "TaskHandle" || name == "Output" || name == "Failure"
}
