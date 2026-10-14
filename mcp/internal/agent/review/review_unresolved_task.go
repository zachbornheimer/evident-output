package review

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// taskResolvingMethods are the Task methods after which DOM-021 stays quiet.
// Define is the only one that gives a Task work: Verify, Problem, Fact and
// Writer decorate a Task that still has nothing to run. The rest are removed
// spellings that resolved a Task in older releases; dirty input that still
// uses them is reported by API-032, and DOM-021 does not pile a second
// finding on the same line.
var taskResolvingMethods = map[string]bool{
	"Define": true,
	"Done":   true, "Fail": true, "Failf": true, "Block": true, "Blockf": true, "Skipped": true,
	"Kept": true, "Cancel": true, "Add": true, "Create": true, "Delete": true, "Push": true,
	"Remove": true, "Update": true, "Write": true, "Run": true, "Each": true, "Go": true,
}

// detectUnresolvedTask is DOM-021: a Task declared in a function that
// neither Defines it nor hands it on. Wiring its Writer or adding Facts
// gives it no work, so the Task ends NotStarted and the run concludes
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
				Message:    "Task " + bound.Name + " is declared but never Defined or handed on; it never runs and the run concludes partial",
				File:       filename,
				Line:       fset.Position(bound.Pos()).Line,
				Suggestion: "run the work inside " + bound.Name + ".Define(func(ctx context.Context) error { ... }) and return nil, evo.Fail, evo.Refuse or evo.Exclude",
			})
		}
	}
	return findings
}

// declaredTasks is every identifier body binds with := to a Task(...)
// chain that does not already Define the Task itself.
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

// chainResolvesTask reports whether a Task(...) chain already calls Define,
// as in r.Task("x").Define(fn).
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

// taskHandleBuilders are the Task methods that return the same Task, so a
// chain of them used as a value (passed, returned, stored) hands the Task on
// just as the bare variable would. Bytes, Key, Next, NextCommand, Step and
// Warn are removed spellings that dirty input still chains.
var taskHandleBuilders = map[string]bool{
	"After": true, "Define": true, "Doing": true, "Fact": true, "Problem": true,
	"Progress": true, "Summary": true, "Verify": true,
	"Bytes": true, "Key": true, "Next": true, "NextCommand": true, "Step": true, "Warn": true,
}

// taskSettledIn reports whether body Defines the Task bound to name or
// lets it escape. Each use of name is read as the whole method chain it
// roots, so fetch.After(x).Define(fn) Defines fetch. A chain settles the
// Task when any link is Define, or when the bare variable or a chain of
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
