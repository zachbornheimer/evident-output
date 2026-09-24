// Package review — API-062: Kept/Skipped record a Task's own disposition
// (the item is the Task, docs/reference.md) and resolve it, so calling
// either again on the same Task is misuse. The per-item shape is
// group.Task(item).Kept(reason); the renderer folds those children into
// one tally under the Group's row (contract §25).
//
// Detection is structural. A disposition call repeats when its receiver
// (a plain identifier or selector, never a call such as group.Task(x))
// either:
//   - sits inside a loop that did not bind it (by :=, =, var, or range),
//     so it runs once per iteration on one Task; or
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

// dispositionMethods are the TaskHandle verbs that record the Task's own
// disposition and resolve it.
var dispositionMethods = map[string]bool{"Kept": true, "Skipped": true}

// detectRepeatedDisposition is API-062.
func detectRepeatedDisposition(filename string, file *ast.File, fset *token.FileSet) []Finding {
	scan := repeatedDispositionScan{filename: filename, fset: fset, reported: map[token.Pos]bool{}}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			scan.visitStatementList(node.List)
		case *ast.CaseClause:
			scan.visitStatementList(node.Body)
		case *ast.ForStmt:
			scan.visitLoop(node.Body, node.Init)
		case *ast.RangeStmt:
			scan.visitLoop(node.Body, node)
		}
		return true
	})
	return scan.findings
}

type repeatedDispositionScan struct {
	filename string
	fset     *token.FileSet
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
		call, recv, ok := dispositionCall(expr.X)
		if !ok {
			continue
		}
		if seen[recv] {
			s.report(call, recv)
		}
		seen[recv] = true
	}
}

// visitLoop flags a disposition call anywhere in body whose receiver the
// loop itself (its header or its body) did not bind. A nested loop is its
// own scope: detectRepeatedDisposition visits it separately, against its
// own bindings.
func (s *repeatedDispositionScan) visitLoop(body *ast.BlockStmt, header ast.Node) {
	bound := boundNames(body)
	if header != nil {
		for name := range boundNames(header) {
			bound[name] = true
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if _, isFunc := n.(*ast.FuncLit); isFunc {
			return false // a callback body runs on its own schedule
		}
		if isLoop(n) {
			return false
		}
		call, recv, ok := dispositionCall(n)
		if ok && !bound[rootIdent(call.Fun.(*ast.SelectorExpr).X)] {
			s.report(call, recv)
		}
		return true
	})
}

func (s *repeatedDispositionScan) report(call *ast.CallExpr, recv string) {
	if s.reported[call.Pos()] {
		return
	}
	s.reported[call.Pos()] = true
	method := call.Fun.(*ast.SelectorExpr).Sel.Name
	pos := s.fset.Position(call.Pos())
	s.findings = append(s.findings, Finding{
		RuleID:   "API-062",
		Severity: "warning",
		Message:  recv + "." + method + " is called more than once on one Task; " + method + " records that Task's own disposition and resolves it",
		File:     s.filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Suggestion: "declare one Task per item and record its disposition there: group.Task(item)." + method +
			"(reason) — evo folds the Group's item children into one tally under its row",
		RequiredVersion: dialectOneZero,
	})
}

// dispositionCall reports whether n is recv.Kept(reason) or
// recv.Skipped(reason) on a named receiver (never a call such as
// group.Task(item), which is a fresh Task each time).
func dispositionCall(n ast.Node) (*ast.CallExpr, string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !dispositionMethods[sel.Sel.Name] || rootIdent(sel.X) == "" {
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
