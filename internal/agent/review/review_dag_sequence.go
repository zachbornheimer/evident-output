// Package review — EVO-DAG-003's Sequence-membership signal: spec §5 says
// Sequence "simply creates predecessor dependencies automatically", so two
// Tasks declared off the same *evo.SequenceHandle, in declaration order,
// are already ordered with no explicit .After needed (spec §11.6 treats
// Sequence and After as equally authoritative for first-run ordering).
package review

import "go/ast"

// sequenceTaskOrder is the position of a Task within the one evo.Sequence
// that declared it.
type sequenceTaskOrder struct {
	seq   string
	index int
}

// evoSequenceHandleNames returns the identifiers that name an
// *evoPkg.SequenceHandle in file: function parameters typed
// *evoPkg.SequenceHandle, and variables assigned from evoPkg.Sequence(...).
func evoSequenceHandleNames(file *ast.File, evoPkg string) map[string]bool {
	names := map[string]bool{}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Type.Params == nil {
			continue
		}
		for _, field := range fd.Type.Params.List {
			star, ok := field.Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			sel, ok := star.X.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "SequenceHandle" || exprDottedName(sel.X) != evoPkg {
				continue
			}
			for _, name := range field.Names {
				names[name.Name] = true
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		lhs, ok := assign.Lhs[0].(*ast.Ident)
		call, ok2 := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !ok2 || calledFuncDotted(call) != evoPkg+".Sequence" {
			return true
		}
		names[lhs.Name] = true
		return true
	})
	return names
}

// collectSequenceTaskOrder maps each Task variable declared as
// `taskVar := seqVar.Task(...)`, for a seqVar in sequences, to its
// declaration order within that Sequence.
func collectSequenceTaskOrder(file *ast.File, sequences map[string]bool) map[string]sequenceTaskOrder {
	order := map[string]sequenceTaskOrder{}
	nextIndex := map[string]int{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		taskVar, ok := assign.Lhs[0].(*ast.Ident)
		call, ok2 := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !ok2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Task" {
			return true
		}
		seqVar, ok := sel.X.(*ast.Ident)
		if !ok || !sequences[seqVar.Name] {
			return true
		}
		order[taskVar.Name] = sequenceTaskOrder{seq: seqVar.Name, index: nextIndex[seqVar.Name]}
		nextIndex[seqVar.Name]++
		return true
	})
	return order
}

// orderedBySequence reports whether producer is already guaranteed to run
// before consumer because both were declared, in that order, off the same
// evo.Sequence.
func orderedBySequence(order map[string]sequenceTaskOrder, producer, consumer string) bool {
	p, ok := order[producer]
	if !ok {
		return false
	}
	c, ok := order[consumer]
	if !ok {
		return false
	}
	return p.seq == c.seq && p.index < c.index
}
