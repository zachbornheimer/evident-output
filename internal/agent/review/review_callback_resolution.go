// Package review — API-040 and FP-006: a Define callback that resolves its
// own row twice (Fail is current; Failf was removed in 1.1), and
// Doing-then-Done theater (Done was removed in 1.1; the
// detector still recognizes the legacy shape).
package review

import (
	"go/ast"
	"go/token"
)

// ===== API-040: Fail inside a Define/mutation callback whose result
// is returned, directly or one call away (zq app.go:308-350's executeCommand,
// reached from runParallel's Define at app.go:155-176), when that Fail adds
// nothing beyond the returned error. A Fail carrying Detail/Next/NextCommand
// is the canonical diagnostic form: the returned error stays the failure
// Wait() reports (ZYS-1182 owner decision A). Failf was removed in 1.1.

func detectFailInResolvedCallback(filename string, file *ast.File, fset *token.FileSet) []Finding {
	funcs := map[string]*ast.BlockStmt{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
			funcs[fd.Name.Name] = fd.Body
		}
		return true
	})

	var findings []Finding
	visited := map[*ast.BlockStmt]bool{}
	var visit func(block *ast.BlockStmt, depth int)
	visit = func(block *ast.BlockStmt, depth int) {
		if block == nil || visited[block] || depth > 2 {
			return
		}
		visited[block] = true
		findings = append(findings, scanBlockForFailfReturn(filename, block, fset)...)
		ast.Inspect(block, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if body, ok := funcs[calledFuncName(call)]; ok {
				visit(body, depth+1)
			}
			return true
		})
	}
	for _, fl := range evoResolutionCallbacks(file) {
		visit(fl.Body, 0)
	}
	return findings
}

// scanBlockForFailfReturn recurses through a block's own control-flow
// (if/for/range/switch), never into a nested FuncLit, looking for the two
// double-resolve shapes: `return task.Fail(...)` and `task.Fail(...)`
// immediately followed by `return <non-nil err>` where the Fail adds nothing
// beyond the returned error. Failf was removed in 1.1 and remains dirty input.
func scanBlockForFailfReturn(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	stmts := block.List
	for i, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			if len(s.Results) != 1 {
				continue
			}
			call, ok := s.Results[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			// Block then return nil is how a Define refuses (E-105). Failf
			// was removed in 1.1; only Fail restates what the returned
			// error already does.
			if !ok || sel.Sel.Name != "Failf" || !isLikelyEvoReceiver(sel.X) {
				continue
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, failResolvedInCallbackFinding(filename, pos, exprDottedName(sel.X), sel.Sel.Name, "return"))
		case *ast.ExprStmt:
			call, ok := s.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Fail" && sel.Sel.Name != "Block") || !isLikelyEvoReceiver(sel.X) {
				continue
			}
			if failAddsDiagnostics(call) {
				continue // Fail or Block(summary, options...) then return err: the returned error stays the failure Wait() reports
			}
			if i+1 >= len(stmts) {
				continue
			}
			ret, ok := stmts[i+1].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			id, ok := ret.Results[0].(*ast.Ident)
			if !ok || id.Name == "nil" {
				continue
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, failResolvedInCallbackFinding(filename, pos, exprDottedName(sel.X), sel.Sel.Name, "statement"))
		case *ast.IfStmt:
			findings = append(findings, scanBlockForFailfReturn(filename, s.Body, fset)...)
			findings = append(findings, scanIfElseForFailfReturn(filename, s.Else, fset)...)
		case *ast.ForStmt:
			findings = append(findings, scanBlockForFailfReturn(filename, s.Body, fset)...)
		case *ast.RangeStmt:
			findings = append(findings, scanBlockForFailfReturn(filename, s.Body, fset)...)
		case *ast.SwitchStmt:
			for _, c := range s.Body.List {
				if cc, ok := c.(*ast.CaseClause); ok {
					findings = append(findings, scanBlockForFailfReturn(filename, &ast.BlockStmt{List: cc.Body}, fset)...)
				}
			}
		}
	}
	return findings
}

// failAddsDiagnostics reports whether a Fail call carries anything beyond its
// summary: a Detail, Next, NextCommand, or any other ProblemOption. Such a Fail
// records structured diagnostics and the callback still returns the error, so
// Wait() and callers see the real failure (ZYS-1182 owner decision A).
func failAddsDiagnostics(call *ast.CallExpr) bool {
	return len(call.Args) > 1
}

func scanIfElseForFailfReturn(filename string, els ast.Stmt, fset *token.FileSet) []Finding {
	switch e := els.(type) {
	case *ast.BlockStmt:
		return scanBlockForFailfReturn(filename, e, fset)
	case *ast.IfStmt:
		findings := scanBlockForFailfReturn(filename, e.Body, fset)
		return append(findings, scanIfElseForFailfReturn(filename, e.Else, fset)...)
	default:
		return nil
	}
}

func failResolvedInCallbackFinding(filename string, pos token.Position, recv, verb, shape string) Finding {
	suggestion := "return the error; do not call " + verb + " first"
	if recv != "" {
		suggestion = "return the error and delete the bare " + recv + "." + verb + "(...) call (Define resolves the task from the returned error), " +
			"or keep it and give it something to say: " + recv + "." + verb + "(\"<summary>\", evo.Detail(err.Error()), evo.NextCommand(...)); return err"
	}
	return Finding{
		RuleID:     "API-040",
		Message:    "the callback resolves the task; " + verb + " that adds no Detail or remedy only restates the returned error (" + shape + " form)",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: suggestion,
	}
}

// ===== FP-006: Doing(...) immediately followed by Done(...) on the same
// handle with no Define submitting work between them — the
// theater FP-005's old suggestion prescribed (zq fix.go:58,265).

func detectDoingDoneTheater(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	// Same-statement chain: X.Doing(...).Done(...).
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Done" {
			return true
		}
		doingCall, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		doingSel, ok := doingCall.Fun.(*ast.SelectorExpr)
		if !ok || doingSel.Sel.Name != "Doing" || !isLikelyEvoReceiver(doingSel.X) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, doingDoneTheaterFinding(filename, pos, exprDottedName(doingSel.X)))
		return true
	})
	// Adjacent statements on the same handle, only non-evo work between.
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		findings = append(findings, scanDoingDoneAdjacent(filename, fn.Body, fset, map[string]bool{})...)
		return false
	})
	return findings
}

func scanDoingDoneAdjacent(filename string, block *ast.BlockStmt, fset *token.FileSet, pending map[string]bool) []Finding {
	var findings []Finding
	for _, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.ExprStmt:
			call, ok := s.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			recv := exprDottedName(sel.X)
			if recv == "" {
				continue
			}
			switch sel.Sel.Name {
			case "Doing":
				pending[recv] = true
			case "Done":
				if pending[recv] {
					pos := fset.Position(call.Pos())
					findings = append(findings, doingDoneTheaterFinding(filename, pos, recv))
				}
				delete(pending, recv)
			case "Define":
				delete(pending, recv)
			}
		case *ast.IfStmt:
			findings = append(findings, scanDoingDoneAdjacent(filename, s.Body, fset, pending)...)
		case *ast.ForStmt:
			findings = append(findings, scanDoingDoneAdjacent(filename, s.Body, fset, pending)...)
		case *ast.RangeStmt:
			findings = append(findings, scanDoingDoneAdjacent(filename, s.Body, fset, pending)...)
		}
	}
	return findings
}

func doingDoneTheaterFinding(filename string, pos token.Position, recv string) Finding {
	suggestion := "replace Doing(...).Done(...) with Define(func(ctx context.Context) error { ... })"
	if recv != "" {
		suggestion = "replace " + recv + ".Doing(...) ... " + recv + ".Done(...) with " + recv + ".Define(func(ctx context.Context) error { ... })"
	}
	return Finding{
		RuleID:     "FP-006",
		Message:    "Doing(...) is immediately followed by Done(...) with no Define submitting work between them; the row narrates work that already happened off-screen",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: suggestion,
	}
}
