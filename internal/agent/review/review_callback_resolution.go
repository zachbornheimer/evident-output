// Package review — API-040 and FP-006: a Define callback that resolves its
// own row twice, and Doing-then-Done theater (Done was removed in 1.1; the
// detector still recognizes the legacy shape).
package review

import (
	"go/ast"
	"go/token"
)

// ===== API-040: Fail inside a Define/mutation callback whose result is also
// returned, directly or one call away (zq app.go:308-350's executeCommand,
// reached from runParallel's Define at app.go:155-176). This does not
// double-resolve the row (a callback that self-resolves and then returns
// the same outcome is documented, safe behavior — Define never restates a
// terminal task), but it is redundant ceremony: Define's own "non-nil
// return fails the task" already does what the explicit Fail call did
// (evo-dialect-axes-report.md axis 3/6/12). Block is exempt: it is the only
// way to conclude the Task Blocked, so a Block statement followed by a
// returned error is the correct shape, not flagged here.

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
		findings = append(findings, scanBlockForFailReturn(filename, block, fset)...)
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

// scanBlockForFailReturn recurses through a block's own control-flow
// (if/for/range/switch), never into a nested FuncLit, looking for the
// redundant-resolve shape: `task.Fail(...)` immediately followed by
// `return <non-nil err>`. Block is exempt (see the ===== API-040 comment
// above): it is the only way to conclude the Task Blocked, so a Block
// statement followed by a returned error is left alone.
func scanBlockForFailReturn(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	stmts := block.List
	for i, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ExprStmt:
			call, ok := s.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Fail" || !isLikelyEvoReceiver(sel.X) {
				continue
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
			findings = append(findings, failResolvedInCallbackFinding(filename, pos, exprDottedName(sel.X)))
		case *ast.IfStmt:
			findings = append(findings, scanBlockForFailReturn(filename, s.Body, fset)...)
			findings = append(findings, scanIfElseForFailReturn(filename, s.Else, fset)...)
		case *ast.ForStmt:
			findings = append(findings, scanBlockForFailReturn(filename, s.Body, fset)...)
		case *ast.RangeStmt:
			findings = append(findings, scanBlockForFailReturn(filename, s.Body, fset)...)
		case *ast.SwitchStmt:
			for _, c := range s.Body.List {
				if cc, ok := c.(*ast.CaseClause); ok {
					findings = append(findings, scanBlockForFailReturn(filename, &ast.BlockStmt{List: cc.Body}, fset)...)
				}
			}
		}
	}
	return findings
}

func scanIfElseForFailReturn(filename string, els ast.Stmt, fset *token.FileSet) []Finding {
	switch e := els.(type) {
	case *ast.BlockStmt:
		return scanBlockForFailReturn(filename, e, fset)
	case *ast.IfStmt:
		findings := scanBlockForFailReturn(filename, e.Body, fset)
		return append(findings, scanIfElseForFailReturn(filename, e.Else, fset)...)
	default:
		return nil
	}
}

func failResolvedInCallbackFinding(filename string, pos token.Position, recv string) Finding {
	suggestion := "replace with `return err` (or the wrapped error) and delete the Fail(...) call; Define resolves the task from the returned error"
	if recv == "" {
		suggestion = "return the error; do not call Fail first"
	}
	return Finding{
		RuleID:     "API-040",
		Message:    "the callback resolves the task; return the error, do not Fail first (statement form is redundant once Define resolves it from the returned error)",
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
