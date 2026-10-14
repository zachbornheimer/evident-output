// Package review — EVO-FILE-001 and EVO-EXEC-001: hand-rolled
// file/process reconciliation that evo.File/evo.Exec exist to replace, plus
// the honest freshness-boundary warning spec §7 documents (tracking an
// output after arbitrary work cannot retroactively skip that work).
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// freshnessCheckNameHint is the substring set a raw exec's guarding
// condition commonly spells out ("outputIsStale", "needsRebuild", ...) —
// deliberately narrow so an unrelated boolean-returning helper is a miss,
// not a false positive.
var freshnessCheckNameHints = []string{"stale", "fresh", "outdated", "rebuild", "uptodate", "up_to_date"}

// detectManualFileReconciliation is EVO-FILE-001: os.WriteFile and
// os.Chmod on the same path within one function — the write/chmod
// boilerplate evo.File's Path/Contents/Mode fields already cover.
func detectManualFileReconciliation(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		writes := map[string]token.Pos{}
		chmods := map[string]bool{}
		walkOwnBody(body, func(n ast.Node) {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != "os" || len(call.Args) == 0 {
				return
			}
			key := exprKey(call.Args[0])
			if key == "" {
				return
			}
			switch sel.Sel.Name {
			case "WriteFile":
				writes[key] = call.Pos()
			case "Chmod":
				chmods[key] = true
			}
		})
		for key, pos := range writes {
			if !chmods[key] {
				continue
			}
			p := fset.Position(pos)
			findings = append(findings, Finding{
				RuleID:     "EVO-FILE-001",
				Message:    "manual file reconciliation can use evo.File",
				File:       filename,
				Line:       p.Line,
				Column:     p.Column,
				Suggestion: "Replace write/chmod/check boilerplate with one declarative evo.File(ctx, evo.FileSpec{...}) call",
			})
		}
	})
	return findings
}

// detectExpensiveWorkBeforeFileOp is EVO-FILE-001's freshness-boundary
// warning: a callback that calls out to a local function before its
// trailing evo.File/evo.Exec return already paid that cost — tracking the
// operation cannot retroactively skip work that already ran (spec §7).
// Only a call threaded with a ctx argument is in scope: passing context is
// the idiomatic Go signal for I/O-bound or cancelable work, which is what
// can plausibly be expensive enough to matter here — a cheap pure helper
// (formatting a key, deriving a path) never needs one and is not flagged.
func detectExpensiveWorkBeforeFileOp(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		if len(body.List) < 2 {
			return
		}
		last, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
		if !ok || len(last.Results) != 1 || !isEvoOperationCall(last.Results[0], pkg) {
			return
		}
		for _, stmt := range body.List[:len(body.List)-1] {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for _, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				callee, ok := call.Fun.(*ast.Ident)
				if !ok || !callThreadsContext(call) {
					continue
				}
				pos := fset.Position(call.Pos())
				findings = append(findings, Finding{
					RuleID: "EVO-FILE-001",
					// The freshness-boundary variant is deliberately stronger than
					// EVO-FILE-001's catalog suggestion: work already ran that the
					// trailing File/Exec cannot skip. The only catalog override.
					Severity:   "warning",
					Message:    callee.Name + "(...) already ran before the trailing evo.File/evo.Exec call; tracking the operation cannot retroactively skip work that already executed",
					File:       filename,
					Line:       pos.Line,
					Column:     pos.Column,
					Suggestion: "run the expensive work as an evo.Exec that declares its Inputs and Outputs, so Evo skips it while they are current, or guard it with the Task's Verify",
				})
			}
		}
	})
	return findings
}

// callThreadsContext reports whether call passes a context-named identifier
// argument ("ctx" or "context") — the idiomatic marker of I/O-bound or
// cancelable work, and this detector's proxy for "plausibly expensive".
func callThreadsContext(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		switch id.Name {
		case "ctx", "context":
			return true
		}
	}
	return false
}

// isEvoOperationCall reports whether expr is a call to File or Exec on the
// evo import named pkg.
func isEvoOperationCall(expr ast.Expr, pkg string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || pkg == "" {
		return false
	}
	name := calledFuncDotted(call)
	return name == pkg+".File" || name == pkg+".Exec"
}

// detectRawExecWithManualFreshness is EVO-EXEC-001: os/exec.Command run
// inside an if-guard whose condition calls an obviously freshness-named
// helper ("outputIsStale", ...) — the raw exec plus hand-rolled staleness
// check evo.Exec's Inputs/Outputs contract already covers.
func detectRawExecWithManualFreshness(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || !callsFreshnessHelper(ifStmt.Cond) {
			return true
		}
		ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name := calledFuncDotted(call); name != "exec.Command" && name != "exec.CommandContext" {
				return true
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, Finding{
				RuleID:     "EVO-EXEC-001",
				Message:    "raw exec with a manual freshness check can use evo.Exec",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: "Replace the raw exec plus manual freshness check with one evo.Exec(ctx, evo.ExecSpec{...}) call that declares Inputs and Outputs",
			})
			return true
		})
		return true
	})
	return findings
}

// callsFreshnessHelper reports whether cond calls a function whose name
// hints at a hand-rolled staleness check (see freshnessCheckNameHints).
func callsFreshnessHelper(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		lower := strings.ToLower(id.Name)
		for _, hint := range freshnessCheckNameHints {
			if strings.Contains(lower, hint) {
				found = true
			}
		}
		return true
	})
	return found
}

// forEachFuncBody visits every named function and function-literal body in
// file — the common shape both file's Define-callback detectors need.
func forEachFuncBody(file *ast.File, visit func(*ast.BlockStmt)) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				visit(fn.Body)
			}
		case *ast.FuncLit:
			if fn.Body != nil {
				visit(fn.Body)
			}
		}
		return true
	})
}

// exprKey renders a cheap, honest identity key for a file-path argument —
// an identifier chain or a string literal's own value — so the same path
// referenced by variable or by literal still matches across two calls.
// Anything else (a call result, a formatted expression) returns "" and is
// skipped rather than guessed at.
func exprKey(e ast.Expr) string {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		if s, err := strconvUnquote(lit.Value); err == nil {
			return "lit:" + s
		}
	}
	if name := exprDottedName(e); name != "" {
		return "id:" + name
	}
	return ""
}
