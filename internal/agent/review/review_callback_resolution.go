// Package review — API-040 and FP-006: a Define callback that resolves its
// own row twice, and Doing-then-Done theater (Done was removed in 1.1; the
// detector still recognizes the legacy shape).
package review

import (
	"go/ast"
	"go/token"
)

// ===== API-040: Failf/Fail inside a Define/mutation callback whose result
// is returned, directly or one call away (zq app.go:308-350's executeCommand,
// reached from runParallel's Define at app.go:155-176). Double-resolves the
// task: Define's own "non-nil return fails" collides with Failf's "resolve
// and return" (evo-dialect-axes-report.md axis 3/6/12).

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
// (if/for/range/switch), never into a nested FuncLit, looking for
// `return task.Failf(...)` — a leftover of the Failf spelling removed in 1.1.
// `task.Fail(...)`/`task.Block(...)` immediately followed by
// `return <non-nil err>` is NOT flagged here since 1.1: Failf/Blockf are
// gone, Fail/Block are statement-form, and folding the wrapped context into
// the summary then returning the same error separately (fmt.Errorf(...);
// task.Fail(wrapped.Error()); return wrapped) is the sanctioned idiom, not a
// double-resolve — Fail/Block terminalize the task immediately, and the
// scheduler's own re-resolution of an already-terminal task on the
// callback's return is a harmless no-op (task_resolve.go's
// IsTerminalTask guard).
func scanBlockForFailfReturn(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	for _, stmt := range block.List {
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
			if !ok || sel.Sel.Name != "Failf" || !isLikelyEvoReceiver(sel.X) {
				continue
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, failResolvedInCallbackFinding(filename, pos, exprDottedName(sel.X), sel.Sel.Name, "return"))
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

// failResolvedInCallbackFinding is API-040's remaining shape: a leftover
// `return task.Failf(...)` call. Failf was removed in 1.1; the migration is
// a plain `return fmt.Errorf(...)`, folding any Fail summary text into the
// error's own message.
func failResolvedInCallbackFinding(filename string, pos token.Position, recv, verb, shape string) Finding {
	suggestion := "replace with `return fmt.Errorf(...)` and the original error, e.g. `return err`; " + verb + " no longer exists (Fail is statement-form since 1.1)"
	if recv != "" {
		suggestion = "replace " + recv + "." + verb + "(\"...\", err) with `return fmt.Errorf(\"...: %w\", err)`, or plain `return err`; " + verb + " was removed in 1.1"
	}
	return Finding{
		RuleID:     "API-040",
		Message:    "the callback resolves the task via the removed " + verb + "; " + shape + " form must migrate to plain fmt.Errorf",
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
