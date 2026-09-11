// Package review — MCP teachability rules (evo-dialect-axes-report.md axis
// 6/12/13/14): every suggestion below names a real, existing spelling. Wait
// is the one pre-approved exception (API-044): TaskHandle.Wait() error is
// landing in parallel with this file and is not yet in this module's public
// API, but its spelling is fixed and it is the correct fix for the
// channel-wait shape API-044 detects.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// mutationVerbNames are TaskHandle's object-first mutation verbs — the
// callback-submitting family Define sits alongside (evo-rec.md "Additions").
var mutationVerbNames = map[string]bool{
	"Create": true, "Delete": true, "Update": true,
	"Add": true, "Remove": true, "Push": true, "Write": true,
}

// evoResolutionCallbacks collects every FuncLit passed directly as the work
// callback of Define or a mutation verb — the two shapes whose return value
// resolves the task through evo's own scheduler (API-040/FP-006's scope).
func evoResolutionCallbacks(file *ast.File) []*ast.FuncLit {
	var out []*ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch {
		case sel.Sel.Name == "Define" && len(call.Args) >= 1:
			if fl, ok := call.Args[0].(*ast.FuncLit); ok {
				out = append(out, fl)
			}
		case mutationVerbNames[sel.Sel.Name] && len(call.Args) >= 2:
			if fl, ok := call.Args[1].(*ast.FuncLit); ok {
				out = append(out, fl)
			}
		}
		return true
	})
	return out
}

// calledFuncName extracts the bare or method name a CallExpr invokes, for
// same-file call-graph lookups that stay honest about their limits (no
// cross-package resolution, no type checking of the receiver).
func calledFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

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
// (if/for/range/switch), never into a nested FuncLit, looking for the two
// double-resolve shapes: `return task.Failf(...)` and `task.Fail(...)`
// immediately followed by `return <non-nil err>`.
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
			if !ok || (sel.Sel.Name != "Failf" && sel.Sel.Name != "Blockf") || !isLikelyEvoReceiver(sel.X) {
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
		suggestion = "replace with `return err` (or the wrapped error) and delete the " + recv + "." + verb + "(...) call; Define/the mutation verb resolves the task from the returned error"
	}
	return Finding{
		RuleID:     "API-040",
		Severity:   "error",
		Message:    "the callback resolves the task; return the error, do not " + verb + " first (" + shape + " form double-resolves under Define)",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: suggestion,
	}
}

// ===== FP-006: Doing(...) immediately followed by Done(...) on the same
// handle with no Define/mutation verb submitting work between them — the
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
			switch {
			case sel.Sel.Name == "Doing":
				pending[recv] = true
			case sel.Sel.Name == "Done":
				if pending[recv] {
					pos := fset.Position(call.Pos())
					findings = append(findings, doingDoneTheaterFinding(filename, pos, recv))
				}
				delete(pending, recv)
			case sel.Sel.Name == "Define" || mutationVerbNames[sel.Sel.Name]:
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
	suggestion := "replace Doing(...).Done(...) with Define(func() error { ... }) or the matching mutation verb"
	if recv != "" {
		suggestion = "replace " + recv + ".Doing(...) ... " + recv + ".Done(...) with " + recv + ".Define(func() error { ... }) or " + recv + "'s matching mutation verb"
	}
	return Finding{
		RuleID:     "FP-006",
		Severity:   "error",
		Message:    "Doing(...) is immediately followed by Done(...) with no Define/mutation verb submitting work between them; the row narrates work that already happened off-screen",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: suggestion,
	}
}

// ===== API-041: a goroutine/.Go(func( closure resolves a predeclared Task
// (Doing/Done/Fail/Progress) with no Define inside it — the scheduler never
// received the work (zq axis-11 P1).

var fanOutResolutionVerbMarkers = []string{".Doing(", ".Done(", ".Fail(", ".Progress("}

func detectGoroutineResolvesPredeclaredTask(filename, src string) []Finding {
	var findings []Finding
	for _, marker := range fanOutClosureMarkers {
		for i := 0; i < len(src); {
			idx := strings.Index(src[i:], marker)
			if idx < 0 {
				break
			}
			idx += i
			body, start, ok := balancedBraceBody(src, idx)
			if !ok {
				break
			}
			if !strings.Contains(body, ".Define(") && containsAnyMarker(body, fanOutResolutionVerbMarkers) {
				findings = append(findings, Finding{
					RuleID:     "API-041",
					Severity:   "error",
					Message:    "goroutine/fan-out closure resolves a predeclared Task (Doing/Done/Fail/Progress) with no Define; evo never received this work to schedule",
					File:       filename,
					Line:       lineAt(src, start),
					Suggestion: "predeclare with Group(...).Each(items) or Group.Task(...), then call task.Define(func() error { ... }) instead of a bare goroutine",
				})
			}
			i = start + len(body)
		}
	}
	return findings
}

func containsAnyMarker(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// ===== API-042: a mutation verb's callback is nil, or a no-op — the work
// already happened elsewhere and the callback is theater over it (zq
// README.md:39's Create(..., nil, ...), setup_python.go:172-181's
// Create("module", func() error { return installedPythonModuleCount(...) })).

func detectNoOpMutationCallback(filename string, file *ast.File, fset *token.FileSet) []Finding {
	funcs := map[string]*ast.FuncDecl{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
			funcs[fd.Name.Name] = fd
		}
		return true
	})

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !mutationVerbNames[sel.Sel.Name] || len(call.Args) < 2 {
			return true
		}
		recv := exprDottedName(sel.X)
		pos := fset.Position(call.Pos())
		switch arg := call.Args[1].(type) {
		case *ast.Ident:
			if arg.Name == "nil" {
				findings = append(findings, noOpMutationFinding(filename, pos, recv, sel.Sel.Name, "nil"))
				return true
			}
			if fd, ok := funcs[arg.Name]; ok && funcBodyLooksLikeNoOpWork(fd.Body) {
				findings = append(findings, noOpMutationFinding(filename, pos, recv, sel.Sel.Name, "named"))
			}
		case *ast.FuncLit:
			if funcBodyIsBareReturnNil(arg.Body) {
				findings = append(findings, noOpMutationFinding(filename, pos, recv, sel.Sel.Name, "literal"))
			} else if name, ok := singleReturnCallName(arg.Body); ok {
				// e.g. Create("module", func() error { return
				// installedPythonModuleCount(name, n) }) — the callback's
				// only statement delegates to a same-file func that itself
				// does no real work (just validates what the caller
				// already computed).
				if fd, ok := funcs[name]; ok && funcBodyLooksLikeNoOpWork(fd.Body) {
					findings = append(findings, noOpMutationFinding(filename, pos, recv, sel.Sel.Name, "named"))
				}
			}
		}
		return true
	})
	return findings
}

// noOpCalleeAllowList are calls cheap enough to still count as "no real
// work" — validation and error construction, not I/O or mutation.
var noOpCalleeAllowList = map[string]bool{
	"fmt.Errorf": true, "errors.New": true, "len": true,
}

func calledFuncDotted(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return exprDottedName(fn.X) + "." + fn.Sel.Name
	default:
		return ""
	}
}

// funcBodyLooksLikeNoOpWork reports whether body's only calls are
// validation/error-construction (the noOpCalleeAllowList) — i.e. it never
// calls out to do the mutation's actual work (zq's installedPythonModuleCount
// only checks a count that was already computed by the caller).
func funcBodyLooksLikeNoOpWork(body *ast.BlockStmt) bool {
	hasRealCall := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !noOpCalleeAllowList[calledFuncDotted(call)] {
			hasRealCall = true
		}
		return true
	})
	return !hasRealCall
}

// singleReturnCallName reports the called function's bare/method name when
// body is exactly one statement, `return someFunc(...)`.
func singleReturnCallName(body *ast.BlockStmt) (string, bool) {
	if len(body.List) != 1 {
		return "", false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return "", false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return "", false
	}
	name := calledFuncName(call)
	return name, name != ""
}

func funcBodyIsBareReturnNil(body *ast.BlockStmt) bool {
	if len(body.List) != 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	id, ok := ret.Results[0].(*ast.Ident)
	return ok && id.Name == "nil"
}

func noOpMutationFinding(filename string, pos token.Position, recv, verb, shape string) Finding {
	message := "mutation verb has a nil callback; the work must run inside the callback"
	if shape != "nil" {
		message = "mutation verb's callback does no real work (only validates/constructs an error); the work already ran elsewhere"
	}
	suggestion := "move the work into the callback, or call " + recv + ".Record(\"" + strings.ToLower(verb) + "\", n, object) when the work already happened"
	if recv == "" {
		suggestion = "move the work into the callback, or call task.Record(verb, n, object) when the work already happened"
	}
	return Finding{
		RuleID:     "API-042",
		Severity:   "error",
		Message:    message,
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: suggestion,
	}
}

// ===== API-043: a plural object literal on a mutation verb — evo pluralizes
// the singular via Affected(n); passing the plural already produces
// "deleted 1 worktrees" (zq axis-14 P17) because an already-plural literal
// round-trips unchanged.

// mutationObjectNouns is the small, deliberately narrow whitelist of object
// nouns this detector recognizes — it only fires when trimming a candidate
// plural suffix yields one of these, so it never guesses at English
// pluralization rules for words it doesn't know (see isSibilantPlural).
var mutationObjectNouns = map[string]bool{
	"worktree": true, "branch": true, "module": true, "package": true,
	"file": true, "directory": true, "dir": true, "tag": true,
	"remote": true, "repo": true, "repository": true, "config": true,
	"lock": true, "cache": true, "session": true, "log": true,
	"artifact": true, "dependency": true, "venv": true, "executable": true,
	"credential": true, "secret": true, "token": true, "key": true,
	"hook": true, "plugin": true, "template": true, "workspace": true,
	"container": true, "job": true, "entry": true, "record": true,
	"snapshot": true, "backup": true, "release": true, "build": true,
	"target": true,
}

func detectPluralMutationObject(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !mutationVerbNames[sel.Sel.Name] || len(call.Args) < 1 {
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
		singular, ok := pluralObjectSingular(text)
		if !ok {
			return true
		}
		recv := exprDottedName(sel.X)
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-043",
			Severity:   "warning",
			Message:    "mutation object literal " + strconv.Quote(text) + " is plural; evo pluralizes the singular via Affected(n)",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace " + strconv.Quote(text) + " with " + strconv.Quote(singular) + " (" + recv + "." + sel.Sel.Name + ")",
		})
		return true
	})
	return findings
}

// pluralObjectSingular returns the singular form when word is a recognized
// plural object noun (mutationObjectNouns), else ("", false).
func pluralObjectSingular(word string) (string, bool) {
	lower := strings.ToLower(word)
	candidates := []string{}
	if strings.HasSuffix(lower, "ies") && len(word) > 3 {
		candidates = append(candidates, word[:len(word)-3]+"y")
	}
	if strings.HasSuffix(lower, "es") && len(word) > 2 {
		candidates = append(candidates, word[:len(word)-2])
	}
	if strings.HasSuffix(lower, "s") && len(word) > 1 {
		candidates = append(candidates, word[:len(word)-1])
	}
	for _, c := range candidates {
		if mutationObjectNouns[strings.ToLower(c)] {
			return c, true
		}
	}
	return "", false
}

// ===== API-044: a hand-rolled channel wrapper around Define reimplements
// TaskHandle.Wait() and hangs when the task is already terminal before
// Define runs (zq setup_python.go:190-210's defineAndWait; axis-3/15
// P15/P16 confirmed hang/deadlock). Wait is pre-approved here though it is
// landing in parallel and not yet in this module's public API — its
// spelling is fixed and it is the correct fix for this shape.

func detectChannelWaitWrapperAroundDefine(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		chanIdx := strings.Index(fn.body, "make(chan error")
		if chanIdx < 0 {
			continue
		}
		if !strings.Contains(fn.body, ".Define(") {
			continue
		}
		if !strings.Contains(fn.body, "<-") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-044",
			Severity:   "error",
			Message:    "a channel-wait wrapper around Define reimplements task.Wait() and hangs when the task is already terminal before Define runs",
			File:       filename,
			Line:       lineAt(src, fn.offset+chanIdx),
			Suggestion: "replace the make(chan error)/Define/<-done wrapper with task.Define(fn); err := task.Wait()",
		})
	}
	return findings
}

// ===== TAX-003: an evo.Reason("literal") used inline as a call argument in
// non-test source, and a reason that merely restates its verb.

// taxonomyReasonVerbWords maps the taxonomy verb to the word it must not be
// restated by its own reason (zq cmd/zq-build/main.go:81's
// Skipped(evo.Reason("skipped"))).
var taxonomyReasonVerbWords = map[string]string{"Skipped": "skipped", "Kept": "kept"}

func detectInlineReasonLiteral(filename string, file *ast.File, fset *token.FileSet) []Finding {
	if strings.HasSuffix(filename, "_test.go") {
		return nil
	}
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		outerSel, _ := call.Fun.(*ast.SelectorExpr)
		for _, arg := range call.Args {
			reasonCall, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := reasonCall.Fun.(*ast.SelectorExpr)
			if !ok || !isEvoIdent(sel.X, pkg) || sel.Sel.Name != "Reason" || len(reasonCall.Args) != 1 {
				continue
			}
			lit, ok := reasonCall.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			pos := fset.Position(reasonCall.Pos())
			findings = append(findings, inlineReasonFinding(filename, pos, pkg, outerSel, text))
		}
		return true
	})
	return findings
}

func inlineReasonFinding(filename string, pos token.Position, pkg string, outerSel *ast.SelectorExpr, text string) Finding {
	if outerSel != nil {
		if verbWord, restates := taxonomyReasonVerbWords[outerSel.Sel.Name]; restates && strings.EqualFold(strings.TrimSpace(text), verbWord) {
			return Finding{
				RuleID:     "TAX-003",
				Severity:   "warning",
				Message:    "reason " + strconv.Quote(text) + " merely restates " + outerSel.Sel.Name + "; a reason names why, not what",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: "replace " + pkg + ".Reason(" + strconv.Quote(text) + ") with the cause, e.g. " + pkg + ".Reason(\"unchanged\")",
			}
		}
	}
	return Finding{
		RuleID:     "TAX-003",
		Severity:   "warning",
		Message:    "inline " + pkg + ".Reason(" + strconv.Quote(text) + ") literal; lift it to a package-level var so it is a compile-time name",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "add `var reason" + exportedReasonName(text) + " = " + pkg + ".Reason(" + strconv.Quote(text) + ")` and use reason" + exportedReasonName(text) + " here",
	}
}

// exportedReasonName turns a reason literal into an exported-style Go
// identifier fragment ("dirty worktree" -> "DirtyWorktree") for the var-name
// this rule's suggestion spells out.
func exportedReasonName(text string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			if upperNext && r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			b.WriteRune(r)
			upperNext = false
		default:
			upperNext = true
		}
	}
	if b.Len() == 0 {
		return "Reason"
	}
	return b.String()
}
