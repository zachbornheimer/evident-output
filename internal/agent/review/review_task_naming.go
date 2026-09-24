// Package review — API-050 and API-045: Task names that describe a phase, category, or bare subject instead of one meaningful action.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// ===== API-050: a Task whose literal name is a generic phase/category word
// (fix/check/classify/resolve/finalize — ZYS-937) sequences two or more
// independently erroring steps in its own Define callback instead of
// performing one action itself. API-045 already flags the bare word on
// sight; this rule adds the structural half of the same distinction — it
// only fires once the callback shows the actual evidence of owning several
// children (zq's own fix/check command family, internal/app/app.go:80's
// a.task("fix", ...)), so it stays silent on a single guarded step under
// the same name and on any multi-step name that already reads as a real
// verb+object action.

// taskPhaseCategoryWords are the generic phase/category labels ZYS-937
// names as suspect: a Task by this name that sequences several independent
// operations is a container wearing one Task's clothes, not one action.
var taskPhaseCategoryWords = map[string]bool{
	"fix": true, "check": true, "classify": true, "resolve": true, "finalize": true,
}

// phaseTaskCallInfo reports whether call is an evo Task("word") call whose
// literal name is a generic phase/category word, returning the matched
// (original-case) word and the call's source position.
func phaseTaskCallInfo(call *ast.CallExpr, fset *token.FileSet) (word string, pos token.Position, ok bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Task" || !isLikelyEvoReceiver(sel.X) || len(call.Args) < 1 {
		return "", token.Position{}, false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", token.Position{}, false
	}
	text, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", token.Position{}, false
	}
	if !taskPhaseCategoryWords[strings.ToLower(strings.TrimSpace(text))] {
		return "", token.Position{}, false
	}
	return text, fset.Position(call.Pos()), true
}

// detectPhaseTaskOwningChildWork covers two shapes: a chained
// X.Task("word").Define(func(){...}) expression, and the same pairing split
// across a local variable (t := X.Task("word") ... t.Define(func(){...})
// later in the same function body) — no cross-function data flow, matching
// this package's other same-block tracking detectors (scanDoingDoneAdjacent).
func detectPhaseTaskOwningChildWork(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" {
			return true
		}
		taskCall, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		word, pos, ok := phaseTaskCallInfo(taskCall, fset)
		if !ok {
			return true
		}
		fl, ok := call.Args[0].(*ast.FuncLit)
		if !ok {
			return true
		}
		if f := phaseTaskDefineFinding(filename, pos, word, fl); f != nil {
			findings = append(findings, *f)
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		findings = append(findings, scanPhaseTaskVarDefines(filename, fn.Body, fset)...)
		return false
	})
	return findings
}

// scanPhaseTaskVarDefines walks block's top-level statements tracking
// `v := recv.Task("word")` assignments where word matches
// taskPhaseCategoryWords, then matches the next `v.Define(func(){...})`
// statement against that pending declaration.
func scanPhaseTaskVarDefines(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	type pendingTask struct {
		word string
		pos  token.Position
	}
	pending := map[string]pendingTask{}
	for _, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
				continue
			}
			id, ok := s.Lhs[0].(*ast.Ident)
			if !ok {
				continue
			}
			call, ok := s.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			if word, pos, ok := phaseTaskCallInfo(call, fset); ok {
				pending[id.Name] = pendingTask{word, pos}
			}
		case *ast.ExprStmt:
			call, ok := s.X.(*ast.CallExpr)
			if !ok || len(call.Args) < 1 {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Define" {
				continue
			}
			recvID, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}
			info, tracked := pending[recvID.Name]
			if !tracked {
				continue
			}
			delete(pending, recvID.Name)
			fl, ok := call.Args[0].(*ast.FuncLit)
			if !ok {
				continue
			}
			if f := phaseTaskDefineFinding(filename, info.pos, info.word, fl); f != nil {
				findings = append(findings, *f)
			}
		}
	}
	return findings
}

// phaseTaskDefineFinding inspects a Define callback's top-level statements
// for 2+ independently erroring steps — the structural evidence that a
// generic phase-named Task (fix/check/classify/resolve/finalize) exists
// primarily to own child-looking work or force a row, rather than perform
// one action itself.
func phaseTaskDefineFinding(filename string, pos token.Position, word string, fl *ast.FuncLit) *Finding {
	steps := countGuardedCallSteps(fl.Body)
	if steps < 2 {
		return nil
	}
	return &Finding{
		RuleID:   "API-050",
		Severity: "warning",
		Message: "Task(" + strconv.Quote(word) + ") sequences " + strconv.Itoa(steps) +
			" independently erroring steps in its own Define callback; it exists primarily to own child-looking work, not to perform one action itself",
		File:   filename,
		Line:   pos.Line,
		Column: pos.Column,
		Suggestion: "replace Task(" + strconv.Quote(word) + ") with Group(" + strconv.Quote(word) +
			") and give each independently erroring step its own verb+object child Task, e.g. group := out.Group(" + strconv.Quote(word) +
			"); group.Task(\"...\").Define(func(ctx context.Context) error { ... })",
	}
}

// countGuardedCallSteps counts block's top-level statements shaped like one
// independent unit of work immediately followed by its own error check —
// `if err := f(...); err != nil { return err }` or `err = f(...)` directly
// followed by `if err != nil { return err }`. It never descends into nested
// control flow or a nested FuncLit, so it only counts steps sequenced
// directly in the callback body, never ones buried inside a loop/branch.
func countGuardedCallSteps(body *ast.BlockStmt) int {
	if body == nil {
		return 0
	}
	steps := 0
	stmts := body.List
	for i := range stmts {
		switch s := stmts[i].(type) {
		case *ast.IfStmt:
			if s.Init != nil && isCallAssign(s.Init) && isErrNeqNilCond(s.Cond) {
				steps++
			}
		case *ast.AssignStmt:
			if !isCallAssign(s) || i+1 >= len(stmts) {
				continue
			}
			next, ok := stmts[i+1].(*ast.IfStmt)
			if ok && next.Init == nil && isErrNeqNilCond(next.Cond) {
				steps++
			}
		}
	}
	return steps
}

// isCallAssign reports whether s is a single-value assignment/definition
// (`err := f(...)` or `err = f(...)`) whose right side is a call.
func isCallAssign(s ast.Stmt) bool {
	assign, ok := s.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return false
	}
	_, ok = assign.Rhs[0].(*ast.CallExpr)
	return ok
}

// isErrNeqNilCond reports whether cond is the bare `err != nil` guard.
func isErrNeqNilCond(cond ast.Expr) bool {
	be, ok := cond.(*ast.BinaryExpr)
	if !ok || be.Op != token.NEQ {
		return false
	}
	id, ok := be.X.(*ast.Ident)
	if !ok || id.Name != "err" {
		return false
	}
	nilIdent, ok := be.Y.(*ast.Ident)
	return ok && nilIdent.Name == "nil"
}

// ===== API-045: Task(name) where name is a bare subject/category label or a
// generic phase word — ZYS-838's "Task means one independently meaningful
// action, not a display row or container". Task is a compile-time-flexible
// spelling (Output/GroupHandle/SequenceHandle.Task all take any string), so
// this boundary cannot be a Go type; zq's own fix/check command family
// (internal/app/app.go:80's a.task("fix", ...), a.task("check", ...)) is the
// canary case that motivated the split into two findings below: a subject
// label is missing its verb, a container word is organizing other work
// wearing one Task's clothes.

// taskSubjectOnlyNames is a narrow, curated list of names ZYS-838 itself
// names as "weak/suspicious" subject labels — not a grammar check (a short
// name can be legitimate in context), only names known to answer "what",
// never "what will this determine".
var taskSubjectOnlyNames = map[string]string{
	"file integrity": "check file integrity",
	"go":             "build Go",
	"ruff":           "lint Python",
	"classify":       "classify staged files",
}

// taskContainerWords are generic phase/category words that organize other
// work rather than being independently meaningful themselves (ZYS-838's
// "fix"/"pre-commit" examples; zq's a.task("fix", ...) command family).
var taskContainerWords = map[string]bool{
	"fix":        true,
	"pre-commit": true,
	"precommit":  true,
	"setup":      true,
	"process":    true,
}

func detectSubjectOnlyOrContainerTaskName(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Task" || len(call.Args) < 1 || !isLikelyEvoReceiver(sel.X) {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		lower := strings.ToLower(strings.TrimSpace(text))
		pos := fset.Position(call.Pos())
		if taskContainerWords[lower] {
			findings = append(findings, containerTaskNameFinding(filename, pos, text))
			return true
		}
		if corrected, ok := taskSubjectOnlyNames[lower]; ok {
			findings = append(findings, subjectOnlyTaskNameFinding(filename, pos, text, corrected))
		}
		return true
	})
	return findings
}

func subjectOnlyTaskNameFinding(filename string, pos token.Position, text, corrected string) Finding {
	return Finding{
		RuleID:   "API-045",
		Severity: "warning",
		Message:  "Task(" + strconv.Quote(text) + ") names a subject, not the work; a Task should name one independently meaningful action",
		File:     filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Suggestion: "rename to Task(" + strconv.Quote(corrected) + ") — read the name as an action (verb + concrete object) " +
			"that answers what this unit of work will accomplish or determine",
	}
}

func containerTaskNameFinding(filename string, pos token.Position, text string) Finding {
	return Finding{
		RuleID:   "API-045",
		Severity: "warning",
		Message:  "Task(" + strconv.Quote(text) + ") appears to organize several independently meaningful operations, not perform one itself",
		File:     filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Suggestion: "replace Task(" + strconv.Quote(text) + ") with a Group/Sequence such as Group(\"prepare staged files\") " +
			"and give each independently meaningful operation its own verb+object Task underneath",
	}
}
