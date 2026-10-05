package review

import "go/ast"

// taskBindings is which names in one file certainly hold an evo Task:
// parameters, fields, and vars typed *<evo>.TaskHandle (or a bare
// *TaskHandle), and variables assigned from a Task(...) chain. Detectors
// that must prove a receiver is a Task, not merely look like one (API-032's
// removed Done, API-061's removed Record*), ask IsTask.
type taskBindings struct {
	names map[string]bool
}

// newTaskBindings collects every Task binding in file, whose evo import is
// named evoPkg.
func newTaskBindings(file *ast.File, evoPkg string) taskBindings {
	b := taskBindings{names: map[string]bool{}}
	bind := func(name *ast.Ident) { b.names[name.Name] = true }
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			if isTaskHandleType(n.Type, evoPkg) {
				for _, name := range n.Names {
					bind(name)
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if n.Type != nil && isTaskHandleType(n.Type, evoPkg) || i < len(n.Values) && isTaskChain(n.Values[i]) {
					bind(name)
				}
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, rhs := range n.Rhs {
				if id, ok := n.Lhs[i].(*ast.Ident); ok && isTaskChain(rhs) {
					bind(id)
				}
			}
		}
		return true
	})
	return b
}

// IsTask reports whether expr certainly holds an evo Task: a bound
// identifier, a field bound by name (s.task), or a Task(...) chain.
func (b taskBindings) IsTask(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return b.names[e.Name]
	case *ast.SelectorExpr:
		return b.names[e.Sel.Name]
	default:
		return isTaskChain(expr)
	}
}

// isTaskChain reports whether e is a Task(...) call, optionally followed
// by fluent TaskHandle configuration (.Key/.After/.Summary/...).
func isTaskChain(e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == "Task" {
			return true
		}
		e = sel.X
	}
}

// isTaskHandleType reports whether t spells *evoPkg.TaskHandle, or a bare
// *TaskHandle (a dot import, or evo's own package).
func isTaskHandleType(t ast.Expr, evoPkg string) bool {
	star, ok := t.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "TaskHandle" && isEvoIdent(x.X, evoPkg)
	case *ast.Ident:
		return x.Name == "TaskHandle"
	}
	return false
}
