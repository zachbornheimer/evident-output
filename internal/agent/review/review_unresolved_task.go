package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// taskResolvingMethods are the TaskHandle methods that give a Task its
// outcome (or its work): after any of them the row resolves on its own.
// The removed pre-1.1 spellings (Done, the mutation verbs, Run, Each)
// resolved a Task too; a program pinned to an older release still uses
// them, and API-032 already reports them against the current one.
var taskResolvingMethods = map[string]bool{
	"Define": true, "Verify": true, "Fail": true, "Failf": true, "Block": true, "Blockf": true,
	"Skipped": true, "Kept": true, "Cancel": true, "Problem": true,
	"Done": true, "Add": true, "Create": true, "Delete": true, "Push": true, "Remove": true,
	"Update": true, "Write": true, "Run": true, "Each": true, "Go": true,
}

// detectUnresolvedTask is DOM-021: a Task declared in a function that
// neither resolves it nor hands it on. Wiring its Writer or adding Facts
// does not resolve it, so the row stays unresolved and the run concludes
// partial with a misuse hint. Test files are skipped.
func detectUnresolvedTask(filename string, file *ast.File, fset *token.FileSet) []Finding {
	if strings.HasSuffix(filename, "_test.go") {
		return nil // a test declares a Task to probe how an unresolved row renders
	}
	var findings []Finding
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		for _, bound := range declaredTasks(fd.Body) {
			if taskSettledIn(fd.Body, bound.Name) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "DOM-021",
				Message:    "Task " + bound.Name + " is declared but never Defined, resolved, or handed on; its row stays unresolved and the run concludes partial",
				File:       filename,
				Line:       fset.Position(bound.Pos()).Line,
				Suggestion: "run the work inside " + bound.Name + ".Define(func(ctx context.Context) error { ... }), or resolve it with Fail, Block, or Skipped",
			})
		}
	}
	return findings
}

// declaredTasks is every identifier body binds with := to a Task(...)
// chain that does not already resolve the Task itself.
func declaredTasks(body *ast.BlockStmt) []*ast.Ident {
	var bound []*ast.Ident
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			id, isIdent := assign.Lhs[i].(*ast.Ident)
			if isIdent && id.Name != "_" && isTaskChain(rhs) && !chainResolvesTask(rhs) {
				bound = append(bound, id)
			}
		}
		return true
	})
	return bound
}

// chainResolvesTask reports whether a Task(...) chain already calls a
// resolving method, as in evo.Task("x").Define(fn).
func chainResolvesTask(e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name == "Task" {
			return false
		}
		if taskResolvingMethods[sel.Sel.Name] {
			return true
		}
		e = sel.X
	}
}

// taskSettledIn reports whether body resolves the Task bound to name or
// lets it escape: any use of name other than as a method receiver
// (returned, passed, stored, sent) hands it to code this function cannot
// see.
func taskSettledIn(body *ast.BlockStmt, name string) bool {
	settled := false
	receivers := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if settled {
			return false
		}
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && id.Name == name {
				receivers[id] = true
				settled = taskResolvingMethods[n.Sel.Name]
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					receivers[id] = true // the binding itself, not a use
				}
			}
		case *ast.Ident:
			settled = n.Name == name && !receivers[n]
		}
		return true
	})
	return settled
}
