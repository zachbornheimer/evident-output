// Package review — API-062: Skipped records a Task's own disposition
// (the item is the Task, docs/reference.md) and resolves it, so calling
// it again on the same Task is misuse. The per-item shape is
// group.Task(item).Skipped(reason); the renderer folds those children into
// one tally under the Group's row (contract §25).
//
// Detection is structural. A disposition call counts only when its
// receiver certainly holds an evo Task (see taskBindings) and is a plain
// identifier or selector, never a call such as group.Task(x). It repeats
// when that receiver either:
//   - sits inside a loop that reruns it without binding it (by :=, =,
//     var, or range), so it runs once per iteration on one Task (see
//     loopChain.reruns for how return, break, continue and goto decide
//     which loop, if any, reruns it); or
//   - already received a disposition call earlier in the same statement
//     list, with no assignment rebinding it in between, so both run in
//     sequence on one Task.
//
// Calls on exclusive branches (if/else) sit in different statement lists
// and stay silent.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
)

// dispositionMethods are the TaskHandle verb that records the Task's own
// disposition and resolves it.
var dispositionMethods = map[string]bool{"Skipped": true}

// detectRepeatedDisposition is API-062.
func detectRepeatedDisposition(filename string, file *ast.File, fset *token.FileSet) []Finding {
	scan := repeatedDispositionScan{
		filename: filename,
		fset:     fset,
		tasks:    newTaskBindings(file, evoImportName(file)),
		reported: map[token.Pos]bool{},
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			scan.visitStatementList(node.List)
		case *ast.CaseClause:
			scan.visitStatementList(node.Body)
		case *ast.FuncDecl:
			if node.Body != nil {
				scan.visitFunc(node.Body)
			}
		case *ast.FuncLit:
			scan.visitFunc(node.Body)
		}
		return true
	})
	return scan.findings
}

type repeatedDispositionScan struct {
	filename string
	fset     *token.FileSet
	tasks    taskBindings
	reported map[token.Pos]bool
	findings []Finding
}

// visitStatementList flags the second disposition call on one receiver
// among a run of sibling statements.
func (s *repeatedDispositionScan) visitStatementList(stmts []ast.Stmt) {
	seen := map[string]bool{}
	for _, stmt := range stmts {
		if assign, ok := stmt.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				delete(seen, types.ExprString(lhs)) // a fresh Task from here on
			}
			continue
		}
		expr, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, recv, ok := s.dispositionCall(expr.X)
		if !ok {
			continue
		}
		if seen[recv] {
			s.report(call, recv)
		}
		seen[recv] = true
	}
}

// visitFunc flags a disposition call that a loop around it reruns on
// one Task. A function literal inside body is its own function, visited on
// its own: a callback body runs on its own schedule.
func (s *repeatedDispositionScan) visitFunc(body *ast.BlockStmt) {
	s.visitLoops(body, nil, labelSpans(body))
}

// visitLoops walks n with loops as the loops around it, outermost first.
func (s *repeatedDispositionScan) visitLoops(n ast.Node, loops loopChain, labels map[string]nodeSpan) {
	ast.Inspect(n, func(child ast.Node) bool {
		if child == n {
			return true
		}
		switch c := child.(type) {
		case *ast.FuncLit:
			return false
		case *ast.LabeledStmt:
			if isLoop(c.Stmt) {
				s.visitLoop(c.Stmt, c.Label.Name, loops, labels)
				return false
			}
		case *ast.ForStmt, *ast.RangeStmt:
			s.visitLoop(c.(ast.Stmt), "", loops, labels)
			return false
		}
		call, recv, ok := s.dispositionCall(child)
		if ok && loops.reruns(call, rootIdent(call.Fun.(*ast.SelectorExpr).X), labels) {
			s.report(call, recv)
		}
		return true
	})
}

// visitLoop walks one loop's body with the loop pushed onto loops.
func (s *repeatedDispositionScan) visitLoop(loop ast.Stmt, label string, loops loopChain, labels map[string]nodeSpan) {
	inner := append(loops[:len(loops):len(loops)], newLoopScope(loop, label))
	switch l := loop.(type) {
	case *ast.ForStmt:
		s.visitLoops(l.Body, inner, labels)
	case *ast.RangeStmt:
		s.visitLoops(l.Body, inner, labels)
	}
}

func (s *repeatedDispositionScan) report(call *ast.CallExpr, recv string) {
	if s.reported[call.Pos()] {
		return
	}
	s.reported[call.Pos()] = true
	method := call.Fun.(*ast.SelectorExpr).Sel.Name
	pos := s.fset.Position(call.Pos())
	s.findings = append(s.findings, Finding{
		RuleID:  "API-062",
		Message: recv + "." + method + " is called more than once on one Task; " + method + " records that Task's own disposition and resolves it",
		File:    s.filename,
		Line:    pos.Line,
		Column:  pos.Column,
		Suggestion: "declare one Task per item and record its disposition there: group.Task(item)." + method +
			"(reason) — evo folds the Group's item children into one tally under its row",
	})
}

// dispositionCall reports whether n is recv.Skipped(reason) on a named
// receiver that certainly holds an evo Task (never a call such as
// group.Task(item), which is a fresh Task each time, and never another
// type's Skipped).
func (s *repeatedDispositionScan) dispositionCall(n ast.Node) (*ast.CallExpr, string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !dispositionMethods[sel.Sel.Name] || rootIdent(sel.X) == "" || !s.tasks.IsTask(sel.X) {
		return nil, "", false
	}
	return call, types.ExprString(sel.X), true
}

// rootIdent is the leftmost identifier of a plain identifier or selector
// chain (a.b.c -> a), or "" when expr is anything else.
func rootIdent(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.Ident:
			return e.Name
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		default:
			return ""
		}
	}
}

// isLoop reports whether n opens a nested loop scope.
func isLoop(n ast.Node) bool {
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

// boundNames is every identifier n binds by :=, =, var, or a range
// clause's key/value: a receiver bound inside a loop is a fresh value each
// iteration. A nested loop's body is that loop's own scope and is skipped.
func boundNames(n ast.Node) map[string]bool {
	names := map[string]bool{}
	if n == nil {
		return names
	}
	ast.Inspect(n, func(node ast.Node) bool {
		switch d := node.(type) {
		case *ast.AssignStmt:
			if d.Tok == token.DEFINE || d.Tok == token.ASSIGN {
				addIdents(names, d.Lhs...)
			}
		case *ast.ValueSpec:
			for _, id := range d.Names {
				names[id.Name] = true
			}
		case *ast.RangeStmt:
			if d.Tok == token.DEFINE || d.Tok == token.ASSIGN {
				addIdents(names, d.Key, d.Value)
			}
			return false // its body is a nested loop's own scope
		case *ast.ForStmt:
			return false // a nested loop's own scope
		}
		return true
	})
	return names
}

func addIdents(names map[string]bool, exprs ...ast.Expr) {
	for _, e := range exprs {
		if id, ok := e.(*ast.Ident); ok {
			names[id.Name] = true
		}
	}
}
