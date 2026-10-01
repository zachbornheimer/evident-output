package review

import "go/ast"

// localRemedyTypes proves a receiver is NOT an evo TaskHandle or Output: a
// name whose every declaration in the file is a type this file itself declares
// a Next or NextCommand method on (an iterator, a cursor). Anything the file
// cannot show that for stays unproven, so a removed-method call on it is
// reported instead of silently dropped.
type localRemedyTypes struct {
	// withRemedyMethod holds local type names declaring Next or NextCommand.
	withRemedyMethod map[string]bool
	// declared maps a value name to the type names it is declared with in
	// this file; "" marks a declaration whose type is not a plain local name.
	declared map[string][]string
}

func newLocalRemedyTypes(file *ast.File) localRemedyTypes {
	l := localRemedyTypes{withRemedyMethod: map[string]bool{}, declared: map[string][]string{}}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			l.recordMethod(n)
		case *ast.Field:
			l.declare(n.Names, localTypeName(n.Type))
		case *ast.ValueSpec:
			l.declareValueSpec(n)
		case *ast.AssignStmt:
			l.declareDefined(n)
		}
		return true
	})
	return l
}

func (l localRemedyTypes) recordMethod(fn *ast.FuncDecl) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || !isRemedyMethod(fn.Name.Name) {
		return
	}
	if name := localTypeName(fn.Recv.List[0].Type); name != "" {
		l.withRemedyMethod[name] = true
	}
}

func (l localRemedyTypes) declare(names []*ast.Ident, typeName string) {
	for _, name := range names {
		l.declared[name.Name] = append(l.declared[name.Name], typeName)
	}
}

func (l localRemedyTypes) declareValueSpec(spec *ast.ValueSpec) {
	typeName := ""
	if spec.Type != nil {
		typeName = localTypeName(spec.Type)
	}
	l.declare(spec.Names, typeName)
}

func (l localRemedyTypes) declareDefined(assign *ast.AssignStmt) {
	if assign.Tok.String() != ":=" {
		return
	}
	for i, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		typeName := ""
		if len(assign.Lhs) == len(assign.Rhs) {
			typeName = literalTypeName(assign.Rhs[i])
		}
		l.declare([]*ast.Ident{id}, typeName)
	}
}

// proves reports whether x is a name declared only with local types that have
// a Next or NextCommand method of their own.
func (l localRemedyTypes) proves(x ast.Expr) bool {
	var name string
	switch e := x.(type) {
	case *ast.Ident:
		name = e.Name
	case *ast.SelectorExpr:
		name = e.Sel.Name
	default:
		return false
	}
	types := l.declared[name]
	for _, typeName := range types {
		if !l.withRemedyMethod[typeName] {
			return false
		}
	}
	return len(types) > 0
}

// localTypeName is t's name when t is a plain (optionally pointer) identifier.
func localTypeName(t ast.Expr) string {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// literalTypeName is the type of a `T{}` or `&T{}` expression.
func literalTypeName(e ast.Expr) string {
	if unary, ok := e.(*ast.UnaryExpr); ok {
		e = unary.X
	}
	if lit, ok := e.(*ast.CompositeLit); ok && lit.Type != nil {
		return localTypeName(lit.Type)
	}
	return ""
}
