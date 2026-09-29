package review

import (
	"go/ast"
	"go/token"
	"slices"
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

// taskHandleBuilders are the TaskHandle methods that return the same
// handle, so a chain of them used as a value (passed, returned, stored)
// hands the Task on just as the bare variable would.
var taskHandleBuilders = map[string]bool{
	"After": true, "Bytes": true, "Define": true, "Doing": true, "Fact": true, "Key": true,
	"Next": true, "NextCommand": true, "Problem": true, "Progress": true, "Step": true,
	"Summary": true, "Verify": true, "Warn": true,
}

// taskSettledIn reports whether body resolves the Task bound to name or
// lets it escape. Each use of name is read as the whole method chain it
// roots, so fetch.After(x).Define(fn) resolves fetch. A chain settles the
// Task when any link resolves it, or when the bare variable or a chain of
// handle-returning builders is used as a value (returned, passed, stored,
// sent) and so handed to code this function cannot see.
func taskSettledIn(body *ast.BlockStmt, name string) bool {
	settled := false
	var parents []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			parents = parents[:len(parents)-1]
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name && !settled {
			settled = useSettlesTask(id, parents)
		}
		parents = append(parents, n)
		return !settled
	})
	return settled
}

// useSettlesTask climbs from one use of the Task variable through the
// method chain it roots (parents is the path from body down to use) and
// reports whether that chain resolves or hands on the Task.
func useSettlesTask(use *ast.Ident, parents []ast.Node) bool {
	var current ast.Expr = use
	builders := true
	for _, parent := range slices.Backward(parents) {
		switch p := parent.(type) {
		case *ast.SelectorExpr:
			if p.X != current {
				return false // use is the selected name, not a receiver
			}
			if taskResolvingMethods[p.Sel.Name] {
				return true
			}
			builders = builders && taskHandleBuilders[p.Sel.Name]
			current = p
			continue
		case *ast.CallExpr:
			if p.Fun == current {
				current = p
				continue
			}
		case *ast.AssignStmt:
			if current == use && isAssignTarget(p, use) {
				return false // the binding itself, not a use
			}
		case *ast.ExprStmt:
			return false // a statement-level chain that never resolved
		}
		_, isCall := current.(*ast.CallExpr)
		return current == use || (isCall && builders)
	}
	return false
}

// isAssignTarget reports whether id is one of assign's left-hand sides.
func isAssignTarget(assign *ast.AssignStmt, id *ast.Ident) bool {
	for _, lhs := range assign.Lhs {
		if lhs == id {
			return true
		}
	}
	return false
}
