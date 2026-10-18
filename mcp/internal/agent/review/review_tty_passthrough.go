// Package review — TERM-015: a child process wired to the inherited
// terminal while Evo owns it.
package review

import (
	"go/ast"
	"go/token"
)

// detectTTYPassthroughWithoutSuspend flags `cmd.Stdout = os.Stdout` and
// `cmd.Stderr = os.Stderr` in an evo-using file that spawns a process. Two
// processes painting one terminal glue the child's first line to the live
// frame. The child either hands its output to task.Writer() or, when it must
// own the terminal, runs inside Run.Suspend, which pauses the live region.
func detectTTYPassthroughWithoutSuspend(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" || !spawnsProcess(file) {
		return nil
	}
	ended := runEndedRanges(file, pkg)
	suspended := suspendedRanges(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || !assignsInheritedStream(assign) {
			return true
		}
		if inAnyRange(assign.Pos(), suspended) || inAnyRange(assign.Pos(), ended) {
			return true
		}
		pos := fset.Position(assign.Pos())
		findings = append(findings, Finding{
			RuleID:     "TERM-015",
			Message:    "tty-passthrough child (Stdout/Stderr inherited); capture it with task.Writer() so the live row keeps moving, or run a child that needs the terminal inside r.Suspend",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "cmd.Stdout = task.Writer(); cmd.Stderr = task.Writer(); for an interactive child, wrap the whole run in r.Suspend(func() error { ... })",
		})
		return true
	})
	return findings
}

// assignsInheritedStream reports whether assign is `x.Stdout = os.Stdout` or
// `x.Stderr = os.Stderr`.
func assignsInheritedStream(assign *ast.AssignStmt) bool {
	lhs, ok := assign.Lhs[0].(*ast.SelectorExpr)
	if !ok {
		return false
	}
	rhs, ok := assign.Rhs[0].(*ast.SelectorExpr)
	return ok && isOSStdStreamExpr(rhs) && rhs.Sel.Name == lhs.Sel.Name
}

// spawnsProcess reports whether file calls exec.Command or
// exec.CommandContext.
func spawnsProcess(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && identName(sel.X) == "exec" && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
			found = true
		}
		return !found
	})
	return found
}

// suspendedRanges are the bodies of function literals passed to a Suspend
// call: the live region is paused while they run.
func suspendedRanges(file *ast.File) []posRange {
	var spans []posRange
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Suspend" {
			return true
		}
		for _, arg := range call.Args {
			if lit, ok := arg.(*ast.FuncLit); ok {
				spans = append(spans, posRange{lit.Body.Pos(), lit.Body.End()})
			}
		}
		return true
	})
	return spans
}

func inAnyRange(p token.Pos, spans []posRange) bool {
	for _, s := range spans {
		if s.contains(p) {
			return true
		}
	}
	return false
}
