// Package review — EVO-FILE-001 and EVO-EXEC-001 (spec §57): hand-rolled
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
// boilerplate evo.File's Path/Contents/Mode/Basis fields already cover.
func detectManualFileReconciliation(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		writes := map[string]token.Pos{}
		chmods := map[string]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != "os" || len(call.Args) == 0 {
				return true
			}
			key := exprKey(call.Args[0])
			if key == "" {
				return true
			}
			switch sel.Sel.Name {
			case "WriteFile":
				writes[key] = call.Pos()
			case "Chmod":
				chmods[key] = true
			}
			return true
		})
		for key, pos := range writes {
			if !chmods[key] {
				continue
			}
			p := fset.Position(pos)
			findings = append(findings, Finding{
				RuleID:     "EVO-FILE-001",
				Severity:   "suggestion",
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
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		if len(body.List) < 2 {
			return
		}
		last, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
		if !ok || len(last.Results) != 1 || !isEvoOperationCall(last.Results[0]) {
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
					RuleID:     "EVO-FILE-001",
					Severity:   "warning",
					Message:    callee.Name + "(...) already ran before the trailing evo.File/evo.Exec call; tracking the operation cannot retroactively skip work that already executed",
					File:       filename,
					Line:       pos.Line,
					Column:     pos.Column,
					Suggestion: "move the expensive work itself behind an Evo-native operation (evo.Exec) or a lazy/semantic adapter Evo can skip before it runs",
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

// isEvoOperationCall reports whether expr is a call to evo.File or evo.Exec.
func isEvoOperationCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "evo" {
		return false
	}
	return sel.Sel.Name == "File" || sel.Sel.Name == "Exec"
}

// detectRawExecWithManualFreshness is EVO-EXEC-001: os/exec.Command run
// inside an if-guard whose condition calls an obviously freshness-named
// helper ("outputIsStale", ...) — the raw exec plus hand-rolled staleness
// check evo.Exec's Basis/Outputs contract already covers.
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
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != "exec" || sel.Sel.Name != "Command" {
				return true
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, Finding{
				RuleID:     "EVO-EXEC-001",
				Severity:   "suggestion",
				Message:    "raw exec with a manual freshness check can use evo.Exec",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: "Replace the raw exec plus manual freshness check with one evo.Exec(ctx, evo.ExecSpec{...}) call that declares Basis and Outputs",
			})
			return true
		})
		return true
	})
	return findings
}

// detectPatchBasisDropped is EVO-FILE-002: copying only Path/Contents from
// a Patch result into a new FileSpec drops the Patch-time Basis snapshot.
func detectPatchBasisDropped(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		if !funcCallsNamed(body, pkg, "Patch") {
			return
		}
		ast.Inspect(body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isFileSpecLit(lit, pkg) {
				return true
			}
			fields := compositeKV(lit)
			pathExpr, hasPath := fields["Path"]
			contentsExpr, hasContents := fields["Contents"]
			if !hasPath || !hasContents {
				return true
			}
			if _, hasBasis := fields["Basis"]; hasBasis {
				return true
			}
			if !selectorEndsWith(pathExpr, "Path") || !selectorEndsWith(contentsExpr, "Contents") {
				return true
			}
			pos := fset.Position(lit.Pos())
			findings = append(findings, Finding{
				RuleID:          "EVO-FILE-002",
				Severity:        "error",
				Message:         "copying Path/Contents into a new FileSpec drops Patch Basis; pass the PatchResult spec through",
				File:            filename,
				Line:            pos.Line,
				Column:          pos.Column,
				Suggestion:      "pass the PatchResult FileSpec to evo.File(ctx, spec) — do not construct evo.FileSpec{Path: spec.Path, Contents: spec.Contents}",
				RequiredVersion: dialectOneZero,
			})
			return true
		})
	})
	return findings
}

func funcCallsNamed(body *ast.BlockStmt, pkg, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if calledFuncDotted(call) == pkg+"."+name {
			found = true
		}
		return !found
	})
	return found
}

func selectorEndsWith(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
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
