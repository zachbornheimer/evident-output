// Package review — proving a receiver is an evo value. A method name such as
// Summary or Text exists on many unrelated types, so a detector that must not
// fire on them asks whether the receiver is declared as an evo Run, Task,
// Group or Sequence.
package review

import "go/ast"

// evoHandleTypes are the evo types that declare and settle work.
var evoHandleTypes = map[string]bool{"Run": true, "Task": true, "Group": true, "Sequence": true}

// evoHandleReceivers is which names in one file certainly hold an evo Run,
// Task, Group or Sequence: parameters, fields and variables typed as
// *evo.<Type>, variables bound to a Task/Group/Sequence declaration, and
// declaration chains themselves.
type evoHandleReceivers struct {
	typed   map[string]bool
	handles handleScan
}

// newEvoHandleReceivers reads file, whose evo import is named evoPkg.
func newEvoHandleReceivers(file *ast.File, evoPkg string) evoHandleReceivers {
	r := evoHandleReceivers{typed: map[string]bool{}, handles: scanHandles(file, evoPkg)}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			r.bindTyped(n.Names, n.Type, evoPkg)
		case *ast.ValueSpec:
			r.bindTyped(n.Names, n.Type, evoPkg)
		}
		return true
	})
	return r
}

func (r evoHandleReceivers) bindTyped(names []*ast.Ident, typ ast.Expr, evoPkg string) {
	if typ == nil || !isEvoHandleType(typ, evoPkg) {
		return
	}
	for _, name := range names {
		r.typed[name.Name] = true
	}
}

// isEvoHandleType reports whether t spells *evo.Run, *evo.Task, *evo.Group
// or *evo.Sequence.
func isEvoHandleType(t ast.Expr, evoPkg string) bool {
	star, ok := t.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && isEvoIdent(sel.X, evoPkg) && evoHandleTypes[sel.Sel.Name]
}

// Holds reports whether x certainly holds an evo Run, Task, Group or
// Sequence.
func (r evoHandleReceivers) Holds(x ast.Expr) bool {
	switch e := x.(type) {
	case *ast.Ident:
		_, declared := r.handles.handles[e.Name]
		return r.typed[e.Name] || declared
	case *ast.SelectorExpr:
		return r.typed[e.Sel.Name]
	case *ast.ParenExpr:
		return r.Holds(e.X)
	case *ast.CallExpr:
		_, ok := parseHandleChain(e)
		return ok
	}
	return false
}
