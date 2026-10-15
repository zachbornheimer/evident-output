package review

import (
	"go/ast"
	"go/token"
)

// pendingTaskBinding is a variable bound to a Task declaration in one block
// and not yet seen to be Defined or handed on: the statement index of the
// binding, so the statements after it can be searched.
type pendingTaskBinding struct{ index int }

// detectShadowedHandle is DOM-019: a variable bound to a Task is assigned
// another Task declaration before the first Task was Defined or handed on.
// The first Task loses its only handle, never runs, and the run concludes
// partial. DOM-021 cannot see this: a later name.Define settles the name.
func detectShadowedHandle(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		if block, ok := n.(*ast.BlockStmt); ok {
			findings = append(findings, scanBlockForOrphanedTasks(filename, block, fset)...)
		}
		return true
	})
	return findings
}

// scanBlockForOrphanedTasks walks one block's own statements in order. A
// nested block is scanned on its own by detectShadowedHandle, so a Task
// declared in a branch never reads as a reassignment of one declared outside.
func scanBlockForOrphanedTasks(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	pending := map[string]pendingTaskBinding{}
	for i, stmt := range block.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			continue
		}
		for j, lhs := range assign.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			prev, tracked := pending[id.Name]
			if !isTaskChain(assign.Rhs[j]) {
				delete(pending, id.Name)
				continue
			}
			if tracked && !taskSettledIn(&ast.BlockStmt{List: block.List[prev.index+1 : i]}, id.Name) {
				findings = append(findings, Finding{
					RuleID:     "DOM-019",
					Message:    "variable " + id.Name + " is assigned another Task before the Task it held was Defined; the earlier Task never runs and the run concludes partial",
					File:       filename,
					Line:       fset.Position(assign.Pos()).Line,
					Suggestion: "call " + id.Name + ".Define(func(ctx context.Context) error { ... }) before reassigning " + id.Name + ", or give the second declaration its own variable name",
				})
			}
			if chainResolvesTask(assign.Rhs[j]) {
				delete(pending, id.Name)
				continue
			}
			pending[id.Name] = pendingTaskBinding{index: i}
		}
	}
	return findings
}
