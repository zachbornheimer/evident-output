// Package review — MCP teachability rules (evo-dialect-axes-report.md axis
// 6/12/13/14): every suggestion below names a real, existing spelling. Wait
// is the one pre-approved exception (API-044): TaskHandle.Wait() error is
// landing in parallel with this file and is not yet in this module's public
// API, but its spelling is fixed and it is the correct fix for the
// channel-wait shape API-044 detects.
package review

import (
	"go/ast"
	"go/scanner"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// evoResolutionCallbacks collects every FuncLit passed directly as a
// Define callback — the shape whose return value resolves the task through
// evo's own scheduler (API-040/FP-006's scope).
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
		if sel.Sel.Name == "Define" && len(call.Args) >= 1 {
			if fl, ok := call.Args[0].(*ast.FuncLit); ok {
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
		suggestion = "replace with `return err` (or the wrapped error) and delete the " + recv + "." + verb + "(...) call; Define resolves the task from the returned error"
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
		Severity:   "error",
		Message:    "Doing(...) is immediately followed by Done(...) with no Define submitting work between them; the row narrates work that already happened off-screen",
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
					Suggestion: "predeclare with Group.Task(...) (one named Task per item), then call task.Define(func() error { ... }) instead of a bare goroutine",
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

// ===== API-042: an evo.Effect callback is nil, or a no-op — the work
// already happened elsewhere and the callback is theater over it (zq
// README.md:39's nil callback, setup_python.go:172-181's callback that only
// returns installedPythonModuleCount(...)).

// effectCallbackArg is evo.Effect's callback argument index:
// Effect(ctx, spec, fn).
const effectCallbackArg = 2

func detectNoOpEffectCallback(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
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
		if !ok || !isEvoEffectCall(call, pkg) {
			return true
		}
		pos := fset.Position(call.Pos())
		if shape, ok := noOpCallbackShape(call.Args[effectCallbackArg], funcs); ok {
			findings = append(findings, noOpEffectFinding(filename, pos, shape))
		}
		return true
	})
	return findings
}

// isEvoEffectCall reports whether call is pkg.Effect(ctx, spec, fn).
func isEvoEffectCall(call *ast.CallExpr, pkg string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && pkg != "" && isEvoIdent(sel.X, pkg) && sel.Sel.Name == "Effect" && len(call.Args) > effectCallbackArg
}

// noOpCallbackShape classifies fn as a callback that does no real work:
// "nil", a bare `return nil` "literal", or a "named" func (directly or as a
// literal's single delegated return) whose body only validates.
func noOpCallbackShape(fn ast.Expr, funcs map[string]*ast.FuncDecl) (string, bool) {
	switch arg := fn.(type) {
	case *ast.Ident:
		if arg.Name == "nil" {
			return "nil", true
		}
		if fd, ok := funcs[arg.Name]; ok && funcBodyLooksLikeNoOpWork(fd.Body) {
			return "named", true
		}
	case *ast.FuncLit:
		if funcBodyIsBareReturnNil(arg.Body) {
			return "literal", true
		}
		if name, ok := singleReturnCallName(arg.Body); ok {
			if fd, ok := funcs[name]; ok && funcBodyLooksLikeNoOpWork(fd.Body) {
				return "named", true
			}
		}
	}
	return "", false
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

func noOpEffectFinding(filename string, pos token.Position, shape string) Finding {
	message := "evo.Effect has a nil callback; the mutation must run inside the callback"
	if shape != "nil" {
		message = "evo.Effect's callback does no real work (only validates/constructs an error); the mutation already ran elsewhere"
	}
	return Finding{
		RuleID:     "API-042",
		Severity:   "error",
		Message:    message,
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "move the mutation itself inside the Effect callback (or an Evo-native File/Patch); do not report it after the fact with Record/RecordLabel/RecordName — those have no record-only replacement (ZYS-974)",
	}
}

// ===== API-043: a plural EffectSpec.Object literal — evo pluralizes the
// singular from Quantity; passing the plural already produces "deleted 1
// worktrees" (zq axis-14 P17) because an already-plural literal round-trips
// unchanged.

// mutationObjectNouns is the small, deliberately narrow whitelist of Effect object
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

func detectPluralEffectObject(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok || !isEvoEffectSpecLit(cl, pkg) {
			return true
		}
		lit, ok := effectSpecObjectLit(cl)
		if !ok {
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
		pos := fset.Position(lit.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-043",
			Severity:   "warning",
			Message:    "EffectSpec.Object literal " + strconv.Quote(text) + " is plural; evo pluralizes the singular from Quantity",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace Object: " + strconv.Quote(text) + " with Object: " + strconv.Quote(singular),
		})
		return true
	})
	return findings
}

// isEvoEffectSpecLit reports whether cl is a pkg.EffectSpec{...} literal.
func isEvoEffectSpecLit(cl *ast.CompositeLit, pkg string) bool {
	sel, ok := cl.Type.(*ast.SelectorExpr)
	return ok && pkg != "" && isEvoIdent(sel.X, pkg) && sel.Sel.Name == "EffectSpec"
}

// effectSpecObjectLit returns the string literal keyed Object in cl.
func effectSpecObjectLit(cl *ast.CompositeLit) (*ast.BasicLit, bool) {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || identName(kv.Key) != "Object" {
			continue
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		return lit, ok && lit.Kind == token.STRING
	}
	return nil, false
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

// ===== API-052: a caller stores Group/Sequence child Task handles solely to
// loop Wait, filter ErrNotStarted, Snapshot the container, and hand-count
// failed children into its own aggregate error — GroupHandle.Wait/
// SequenceHandle.Wait (ZYS-849) now owns exactly this bookkeeping. zq
// evidence: internal/app/app.go::runParallel (slice of handles, Wait loop,
// Snapshot, failed-child count, "N of N failed" error) and
// internal/app/run_execute.go::waitDefinedRunOperations (Wait loop
// special-casing evo.ErrNotStarted, first-remaining-error return).

// callerWaitLoopSignals are the tokens that, alongside a for-loop calling
// .Wait() on a *TaskHandle, corroborate the container-boilerplate shape
// this rule targets rather than an unrelated Wait() loop (e.g. os/exec's
// Cmd.Wait()) — the rule requires the loop's function reference TaskHandle
// at all, plus at least one of these signals anywhere in the same function.
var callerWaitLoopSignals = []string{"ErrNotStarted", "Snapshot("}

func detectCallerWaitLoopOverContainerChildren(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		// The signature (parameter/receiver types, e.g. "tasks
		// []*evo.TaskHandle") sits before fn.body's opening brace, so the
		// TaskHandle/signal check reads the whole declaration, not only
		// the body statements.
		funcStart := strings.LastIndex(src[:fn.offset], "func ")
		if funcStart < 0 {
			funcStart = fn.offset
		}
		wholeFunc := src[funcStart : fn.offset+len(fn.body)]
		if !strings.Contains(wholeFunc, "TaskHandle") {
			continue
		}
		if !containsAny(wholeFunc, callerWaitLoopSignals) {
			continue
		}
		loop, waitIdx, ok := firstForLoopCallingWait(fn.body)
		if !ok {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-052",
			Severity:   "error",
			Message:    "a caller-owned loop waits on individually stored Task handles, filters ErrNotStarted, snapshots the container, and hand-counts failed children instead of using the container's own Wait",
			File:       filename,
			Line:       lineAt(src, fn.offset+loop.offset+waitIdx),
			Suggestion: "replace the stored-handle Wait loop and hand-counted aggregate error with the owning container's own GroupHandle.Wait()/SequenceHandle.Wait() (e.g. return jobs.Wait())",
		})
	}
	return findings
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// firstForLoopCallingWait finds the first brace-balanced "for" loop in body
// whose block calls .Wait() directly, best-effort via textual scan (mirrors
// allFuncBodies' brace-balanced scan for "func"). It returns the loop's own
// funcBody, the byte offset of ".Wait()" within that loop body, and whether
// a match was found.
func firstForLoopCallingWait(body string) (loop funcBody, waitIdx int, ok bool) {
	scanFrom := 0
	for {
		rel := strings.Index(body[scanFrom:], "for ")
		if rel < 0 {
			return funcBody{}, 0, false
		}
		idx := scanFrom + rel
		if !precededByStatementBoundary(body, idx) {
			scanFrom = idx + len("for ")
			continue
		}
		block, start, balanced := balancedBraceBody(body, idx)
		if !balanced {
			scanFrom = idx + len("for ")
			continue
		}
		if waitIdx := strings.Index(block, ".Wait()"); waitIdx >= 0 {
			return funcBody{body: block, offset: start}, waitIdx, true
		}
		scanFrom = start + len(block)
	}
}

// precededByStatementBoundary reports whether the byte immediately before
// idx starts a new statement (newline, tab, space, or an opening brace) —
// filtering an identifier substring like "before " from matching "for ".
func precededByStatementBoundary(body string, idx int) bool {
	if idx == 0 {
		return true
	}
	switch body[idx-1] {
	case '\n', '\t', ' ', '{':
		return true
	default:
		return false
	}
}

// ===== API-047: a Task/Group/Sequence declaration reuses a sibling literal
// name already used, under the same parent handle, by a different entity
// kind. §3.1's default stable key folds kind into the key
// (kind:parentKey/name), so out.Task("build") and out.Group("build")
// register as two distinct runtime identities that share one visible
// sibling name. failDuplicateSiblingLocked's same-kind check
// (ProblemCodeDuplicateSiblingName) does not catch this because it only
// compares within one kind's own name index (ZYS-944).

// siblingEntityKind names the three declarable entity kinds this rule
// compares across (a package-local mirror of internal/engine's own
// entityKind — review is a static-analysis package and does not import
// engine, so it keeps its own copy of this small vocabulary).
type siblingEntityKind string

const (
	declKindTask     siblingEntityKind = "task"
	declKindGroup    siblingEntityKind = "group"
	declKindSequence siblingEntityKind = "sequence"
)

// siblingDeclKind reports the siblingEntityKind a Task/Group/Sequence declaration
// method name declares, or ("", false) for any other method.
func siblingDeclKind(method string) (kind siblingEntityKind, ok bool) {
	switch method {
	case "Task":
		return declKindTask, true
	case "Group":
		return declKindGroup, true
	case "Sequence":
		return declKindSequence, true
	default:
		return "", false
	}
}

// siblingKindDecl is the first declaration scanBlockForCrossKindSiblings saw
// for one (parent, name) pair: its entity kind and source position, so a
// later declaration under the same parent and name can be compared and
// reported against it.
type siblingKindDecl struct {
	kind siblingEntityKind
	pos  token.Position
}

// detectCrossKindDuplicateSiblingName scans every block independently
// (never across an if/else branch split, where two kinds sharing a name are
// legitimately mutually exclusive) for sibling Task/Group/Sequence
// declarations that reuse one literal name across different kinds under the
// same parent handle.
func detectCrossKindDuplicateSiblingName(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		findings = append(findings, scanBlockForCrossKindSiblings(filename, block, fset)...)
		return true
	})
	return findings
}

// scanBlockForCrossKindSiblings walks block's direct statements without
// descending into a nested BlockStmt — the outer ast.Inspect in
// detectCrossKindDuplicateSiblingName visits and scans a nested block on its
// own, so an if-branch and its else-branch are never compared against each
// other. A chained declaration (out.Task("build").Define(...)) and an
// assigned one (work := out.Group("work")) are both reached because the
// per-statement walk only stops descent at a BlockStmt boundary, never at
// the statement's own expression shape.
func scanBlockForCrossKindSiblings(filename string, block *ast.BlockStmt, fset *token.FileSet) []Finding {
	var findings []Finding
	seen := map[string]map[string]siblingKindDecl{}
	for _, stmt := range block.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if _, ok := n.(*ast.BlockStmt); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			kind, ok := siblingDeclKind(sel.Sel.Name)
			if !ok || len(call.Args) != 1 || !isLikelyEvoReceiver(sel.X) {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(lit.Value)
			if err != nil || name == "" {
				return true
			}
			recv := exprDottedName(sel.X)
			if recv == "" {
				return true
			}
			if seen[recv] == nil {
				seen[recv] = map[string]siblingKindDecl{}
			}
			prior, ok := seen[recv][name]
			if !ok {
				seen[recv][name] = siblingKindDecl{kind: kind, pos: fset.Position(call.Pos())}
				return true
			}
			if prior.kind != kind {
				pos := fset.Position(call.Pos())
				findings = append(findings, crossKindDuplicateSiblingFinding(filename, pos, recv, name, prior, kind))
			}
			return true
		})
	}
	return findings
}

// crossKindDuplicateSiblingFinding builds API-047's Finding: recv/name/kind
// describe the second (flagged) declaration, prior the first one it collides
// with.
func crossKindDuplicateSiblingFinding(filename string, pos token.Position, recv, name string, prior siblingKindDecl, kind siblingEntityKind) Finding {
	method := siblingDeclMethod(kind)
	priorMethod := siblingDeclMethod(prior.kind)
	renamed := strconv.Quote(name + " " + strings.ToLower(method))
	quotedName := strconv.Quote(name)
	return Finding{
		RuleID:   "API-047",
		Severity: "error",
		Message: recv + "." + method + "(" + quotedName + ") reuses the sibling name already declared as a " + string(prior.kind) +
			" at line " + strconv.Itoa(prior.pos.Line) + " (" + recv + "." + priorMethod + "(" + quotedName + ")); the same visible name now names two distinct " +
			string(prior.kind) + "/" + string(kind) + " runtime identities under one parent",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "give " + recv + "." + method + "(" + quotedName + ") its own distinct name, e.g. " + recv + "." + method + "(" + renamed + "), so the two runtime identities that already exist here are no longer visually indistinguishable",
	}
}

// siblingDeclMethod is siblingDeclKind's inverse, so a Finding can name the
// exact method call (out.Group(...), never a bare kind string) the fix
// should read.
func siblingDeclMethod(kind siblingEntityKind) string {
	switch kind {
	case declKindTask:
		return "Task"
	case declKindGroup:
		return "Group"
	case declKindSequence:
		return "Sequence"
	default:
		return string(kind)
	}
}

// ===== API-048: the same Group/Sequence receiver's .Task("literal") called
// more than once with the identical string in one function — declareGroupTask
// fails the second call as a duplicate sibling rather than returning the
// first handle (§3.1; internal/engine/group.go's GroupHandle.Task), so a
// later dependency reference (After, a second Define, ...) must keep the
// first handle instead of re-declaring by name. The contract's own zq prune
// fixture (spec §21/§18) extracts these into a typed var (...) block; that
// is the recommended fix, never required for a Task named only once.

// taskCallSite is one <receiver>.Task("literal") call site, kept in
// declaration order so the finding always lands on the second (repeat)
// occurrence, never the legitimate first declaration.
type taskCallSite struct {
	recv    string
	literal string
	pos     token.Pos
}

func detectRedeclaredTaskLiteral(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		seen := map[string]token.Pos{}
		ast.Inspect(body, func(n ast.Node) bool {
			site, ok := taskCallSiteAt(n)
			if !ok {
				return true
			}
			key := site.recv + "\x00" + site.literal
			if _, dup := seen[key]; dup {
				pos := fset.Position(site.pos)
				findings = append(findings, redeclaredTaskLiteralFinding(filename, pos, site.recv, site.literal))
				return true
			}
			seen[key] = site.pos
			return true
		})
	})
	return findings
}

// taskCallSiteAt reports the <recv>.Task("literal") shape at n, when recv is
// a named identifier (not a chained call result) — a Group/Sequence handle
// held in a variable, the only shape a later reference could re-declare by
// name instead of reusing.
func taskCallSiteAt(n ast.Node) (taskCallSite, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return taskCallSite{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Task" || len(call.Args) != 1 {
		return taskCallSite{}, false
	}
	if _, ok := sel.X.(*ast.Ident); !ok {
		return taskCallSite{}, false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return taskCallSite{}, false
	}
	text, err := strconv.Unquote(lit.Value)
	if err != nil {
		return taskCallSite{}, false
	}
	return taskCallSite{recv: exprDottedName(sel.X), literal: text, pos: call.Pos()}, true
}

func redeclaredTaskLiteralFinding(filename string, pos token.Position, recv, literal string) Finding {
	quoted := strconv.Quote(literal)
	return Finding{
		RuleID:     "API-048",
		Severity:   "suggestion",
		Message:    recv + ".Task(" + quoted + ") is declared again with the same label; the second call fails as a duplicate sibling rather than returning the first handle",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "keep the first " + recv + ".Task(" + quoted + ") handle in a typed variable (a var (...) block when there are several) and reuse it for the later reference instead of re-declaring by name",
	}
}

// ===== API-049: task.Define(func(context.Context) error { ... }) discards
// its scheduler-provided context parameter (unnamed, or named something other
// than the outer "ctx" it shadows) while the body still calls cancellable
// work with the captured outer "ctx" — the row cancels correctly but the
// work it names never observes that cancellation (ZYS-938 / evo-1.x Decisions
// 2026-09-23: "the Define context is authoritative for task
// cancellation/lifecycle").

// defineCallbackContextParamName reports the Define callback's single
// context.Context parameter name ("" for an unnamed or blank-identifier
// parameter) and whether the signature matches func(context.Context) error
// at all.
func defineCallbackContextParamName(ft *ast.FuncType) (string, bool) {
	if ft.Params == nil || len(ft.Params.List) != 1 {
		return "", false
	}
	field := ft.Params.List[0]
	sel, ok := field.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Context" || exprDottedName(sel.X) != "context" {
		return "", false
	}
	if len(field.Names) == 0 {
		return "", true
	}
	name := field.Names[0].Name
	if name == "_" {
		return "", true
	}
	return name, true
}

// bodyCallsWithCapturedIdent reports whether block contains a call passing
// ident as an argument that still resolves to the callback's captured outer
// variable. A local re-declaration of ident (ctx := ..., var ctx ...) in an
// earlier statement of the same or an enclosing block shadows the outer
// variable from that point on — Go's scoping rules start the new binding's
// scope right after the declaring statement — so a call after the shadow
// that passes ident reaches the local, not the ctx Define's callback
// discarded, and is not this rule's shape.
func bodyCallsWithCapturedIdent(block *ast.BlockStmt, ident string) bool {
	return stmtsCallWithCapturedIdent(block.List, ident, false)
}

// stmtsCallWithCapturedIdent walks stmts in declaration order, threading
// whether ident has already been locally shadowed by a preceding statement
// in this same block into every statement (and, for block-bearing
// statements, into their nested blocks) that follows.
func stmtsCallWithCapturedIdent(stmts []ast.Stmt, ident string, shadowed bool) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			if stmtsCallWithCapturedIdent(s.List, ident, shadowed) {
				return true
			}
			continue
		case *ast.IfStmt:
			if ifStmtCallsWithCapturedIdent(s, ident, shadowed) {
				return true
			}
			continue
		case *ast.ForStmt:
			if s.Body != nil && stmtsCallWithCapturedIdent(s.Body.List, ident, shadowed) {
				return true
			}
			continue
		case *ast.RangeStmt:
			if s.Body != nil && stmtsCallWithCapturedIdent(s.Body.List, ident, shadowed) {
				return true
			}
			continue
		case *ast.SwitchStmt:
			if caseClausesCallWithCapturedIdent(s.Body, ident, shadowed) {
				return true
			}
			continue
		case *ast.TypeSwitchStmt:
			if caseClausesCallWithCapturedIdent(s.Body, ident, shadowed) {
				return true
			}
			continue
		case *ast.SelectStmt:
			if commClausesCallWithCapturedIdent(s.Body, ident, shadowed) {
				return true
			}
			continue
		case *ast.LabeledStmt:
			if stmtsCallWithCapturedIdent([]ast.Stmt{s.Stmt}, ident, shadowed) {
				return true
			}
			continue
		}
		if !shadowed && stmtLeafCallsWithIdent(stmt, ident) {
			return true
		}
		if stmtDeclaresLocalIdent(stmt, ident) {
			shadowed = true
		}
	}
	return false
}

func ifStmtCallsWithCapturedIdent(s *ast.IfStmt, ident string, shadowed bool) bool {
	if s.Init != nil {
		if !shadowed && stmtLeafCallsWithIdent(s.Init, ident) {
			return true
		}
		if stmtDeclaresLocalIdent(s.Init, ident) {
			shadowed = true
		}
	}
	if s.Body != nil && stmtsCallWithCapturedIdent(s.Body.List, ident, shadowed) {
		return true
	}
	if s.Else != nil {
		return stmtsCallWithCapturedIdent([]ast.Stmt{s.Else}, ident, shadowed)
	}
	return false
}

func caseClausesCallWithCapturedIdent(body *ast.BlockStmt, ident string, shadowed bool) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		if stmtsCallWithCapturedIdent(cc.Body, ident, shadowed) {
			return true
		}
	}
	return false
}

func commClausesCallWithCapturedIdent(body *ast.BlockStmt, ident string, shadowed bool) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CommClause)
		if !ok {
			continue
		}
		if stmtsCallWithCapturedIdent(cc.Body, ident, shadowed) {
			return true
		}
	}
	return false
}

// stmtDeclaresLocalIdent reports whether stmt is a `ident := ...` or
// `var ident ...` declaration — the two shapes that start a new binding for
// ident, shadowing any outer variable of the same name from here on.
func stmtDeclaresLocalIdent(stmt ast.Stmt, ident string) bool {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok != token.DEFINE {
			return false
		}
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == ident {
				return true
			}
		}
	case *ast.DeclStmt:
		gen, ok := s.Decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			return false
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name == ident {
					return true
				}
			}
		}
	}
	return false
}

// stmtLeafCallsWithIdent reports a call passing ident as an argument
// anywhere within stmt, for a statement kind with no nested block of its
// own sibling statements to thread shadowing through (an ExprStmt, a
// ReturnStmt, ... — nested FuncLit bodies are still descended into, matching
// this rule's original scope of also catching a closure that re-discards
// the same captured ctx).
func stmtLeafCallsWithIdent(stmt ast.Stmt, ident string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			if id, ok := arg.(*ast.Ident); ok && id.Name == ident {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// localDefineReceiverTypes collects every type declared in this file that
// has its own Define(fn func(context.Context) error) method — a shape a
// consumer file may legitimately declare for reasons unrelated to evo (a
// validator, a config builder, a test helper). A call to one of these
// types' Define is not evo's TaskHandle.Define and must not be attributed
// to the scheduler (ZYS-938: proven false positive on such a type sharing a
// file with real evo usage).
func localDefineReceiverTypes(file *ast.File) map[string]bool {
	types := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Name.Name != "Define" {
			continue
		}
		if !isDefineCtxCallbackMethod(fn.Type) {
			continue
		}
		if name := typeNameOf(fn.Recv.List[0].Type); name != "" {
			types[name] = true
		}
	}
	return types
}

// isDefineCtxCallbackMethod reports whether ft is a method signature
// shaped like Define(fn func(context.Context) error) — a single parameter
// that is itself a func(context.Context) error. This is the outer method
// signature (its one parameter names the callback), unlike
// defineCallbackContextParamName, which inspects that inner callback's own
// signature.
func isDefineCtxCallbackMethod(ft *ast.FuncType) bool {
	if ft.Params == nil || len(ft.Params.List) != 1 {
		return false
	}
	fnType, ok := ft.Params.List[0].Type.(*ast.FuncType)
	if !ok {
		return false
	}
	_, isCtxCallback := defineCallbackContextParamName(fnType)
	return isCtxCallback
}

// typeNameOf returns a bare or pointer type expression's identifier name
// ("Validator" for both Validator and *Validator), or "" for anything else
// (a selector into another package, a generic instantiation, ...).
func typeNameOf(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	id, ok := e.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

// identDeclaredTypeName is a best-effort, file-wide scan for a var/:=
// declaration's or a parameter's type for name. It is not scoped to the
// enclosing function — a coarse approximation — but is enough to keep a
// receiver known to be one of localDefineReceiverTypes' non-evo types from
// being mistaken for evo's TaskHandle; it returns "" (no exclusion) for
// anything it cannot resolve, so it only ever narrows this rule, never
// widens it.
func identDeclaredTypeName(file *ast.File, name string) string {
	result := ""
	ast.Inspect(file, func(n ast.Node) bool {
		if result != "" {
			return false
		}
		switch s := n.(type) {
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE || len(s.Lhs) != len(s.Rhs) {
				return true
			}
			for i, lhs := range s.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != name {
					continue
				}
				if t := assignedTypeName(s.Rhs[i]); t != "" {
					result = t
				}
			}
		case *ast.ValueSpec:
			for _, id := range s.Names {
				if id.Name == name && s.Type != nil {
					result = typeNameOf(s.Type)
				}
			}
		case *ast.Field:
			for _, id := range s.Names {
				if id.Name == name {
					result = typeNameOf(s.Type)
				}
			}
		}
		return true
	})
	return result
}

// assignedTypeName names the composite-literal type an expression
// constructs (T{...} or &T{...}), or "" when it constructs anything else.
func assignedTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return typeNameOf(v.Type)
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return assignedTypeName(v.X)
		}
	}
	return ""
}

func detectDefineDiscardsSchedulerContext(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	localTypes := localDefineReceiverTypes(file)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" || len(call.Args) != 1 {
			return true
		}
		if len(localTypes) > 0 {
			if recv, ok := sel.X.(*ast.Ident); ok {
				if t := identDeclaredTypeName(file, recv.Name); t != "" && localTypes[t] {
					return true
				}
			}
		}
		fl, ok := call.Args[0].(*ast.FuncLit)
		if !ok {
			return true
		}
		paramName, isCtxCallback := defineCallbackContextParamName(fl.Type)
		if !isCtxCallback || paramName == "ctx" {
			return true
		}
		if fl.Body == nil || !bodyCallsWithCapturedIdent(fl.Body, "ctx") {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-049",
			Severity:   "error",
			Message:    "Define's callback discards its scheduler-provided context and calls cancellable work with a captured outer ctx instead; the scheduler's cancellation never reaches that work",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "name the callback parameter ctx (func(ctx context.Context) error) and pass that ctx into the work instead of the captured outer variable",
		})
		return true
	})
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

// ===== API-046: task.Skipped(evo.Reason("...")) whose reason text names an
// obvious already-satisfied condition (already up to date, unchanged,
// already latest, already current) rather than true inapplicability (no
// project config, no Go module). Skipped means the check never applied;
// ResolutionAlreadySatisfied — produced by a Verify precondition, or derived
// automatically from evo.File/evo.Exec's own tracked comparison — means the
// check applied and already held. Collapsing the two into Skipped hides a
// real, checked precondition behind the "did not apply" glyph.

// alreadySatisfiedReasonPhrases are multi-word substrings (checked
// case-insensitive against the full reason text) that only ever name a
// checked-and-already-true condition — long enough that they never collide
// with an unrelated sentence.
var alreadySatisfiedReasonPhrases = []string{
	"already up to date", "already up-to-date", "already latest",
	"already current", "already installed", "already exists",
	"already satisfied", "no changes needed", "nothing changed",
	"no update needed", "no upgrade needed",
}

// alreadySatisfiedReasonWords are single bare words that only fire when the
// entire (trimmed) reason text is exactly one of them — a one-word reason
// like "current" or "unchanged" is unambiguous, but the same word inside a
// longer sentence ("current branch is protected") is not, so those go
// through alreadySatisfiedReasonPhrases instead.
var alreadySatisfiedReasonWords = map[string]bool{
	"current": true, "unchanged": true, "latest": true,
	"up to date": true, "up-to-date": true, "uptodate": true,
}

// inapplicabilityReasonPhrases are substrings that name true inapplicability
// (the check never ran because its precondition object doesn't exist) —
// these never fire API-046 even if they also loosely match "current" or
// "up to date" phrasing elsewhere in the same string.
var inapplicabilityReasonPhrases = []string{
	"no project config", "no go module", "no go.mod", "not applicable",
	"n/a", "not a git repo", "not a repository", "no config found",
	"missing config", "no module found", "not present",
}

func detectSkippedForAlreadySatisfied(filename string, file *ast.File, fset *token.FileSet) []Finding {
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
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Skipped" || !isLikelyEvoReceiver(sel.X) || len(call.Args) < 1 {
			return true
		}
		reasonCall, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		reasonSel, ok := reasonCall.Fun.(*ast.SelectorExpr)
		if !ok || !isEvoIdent(reasonSel.X, pkg) || reasonSel.Sel.Name != "Reason" || len(reasonCall.Args) != 1 {
			return true
		}
		lit, ok := reasonCall.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		lower := strings.ToLower(strings.TrimSpace(text))
		if containsAnyMarker(lower, inapplicabilityReasonPhrases) {
			return true
		}
		if !containsAnyMarker(lower, alreadySatisfiedReasonPhrases) && !alreadySatisfiedReasonWords[lower] {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, skippedAlreadySatisfiedFinding(filename, pos, exprDottedName(sel.X), text))
		return true
	})
	return findings
}

func skippedAlreadySatisfiedFinding(filename string, pos token.Position, recv, text string) Finding {
	if recv == "" {
		recv = "task"
	}
	return Finding{
		RuleID:   "API-046",
		Severity: "warning",
		Message:  "Skipped(evo.Reason(" + strconv.Quote(text) + ")) reports \"did not apply\"; the reason names a condition that was checked and already held, which is ResolutionAlreadySatisfied",
		File:     filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Suggestion: "add a precondition check via " + recv + ".Verify(func(ctx context.Context) (bool, error) { ... }) before " + recv +
			".Define(...) so evo resolves ResolutionAlreadySatisfied on its own, or let evo.File/evo.Exec derive it from their own tracked comparison; reserve Skipped for true inapplicability (no project config, no Go module)",
	}
}

// ===== API-060: a TaskHandle.Summary/GroupHandle.Summary literal whose text
// is actually mutation/dry-run/already-satisfied narration (1.1/ZYS-971
// Decisions, 2026-09-23) — Summary is non-terminal result metadata, not a
// replacement for the success-stamp channel Done(text) removed in 1.1.
// That narration belongs to evo.File/evo.Effect's own record,
// ResolutionAlreadySatisfied, or evo.Fact instead.

// summaryStampMarkers are substrings (checked case-insensitive against the
// full Summary text) that only ever narrate a mutation that already
// happened, a dry-run hypothetical, a no-op, or an already-satisfied
// precondition — the exact stamp-style shapes ZYS-971's Decisions name.
var summaryStampMarkers = []string{
	"wrote ", "would add", "would write", "would create", "would delete",
	"would update", "would remove", "nothing to write", "nothing to do",
	"already up to date", "already up-to-date", "already exists",
	"already satisfied", "already installed", "already current",
}

func detectSummaryStampNarration(filename string, file *ast.File, fset *token.FileSet) []Finding {
	if strings.HasSuffix(filename, "_test.go") {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Summary" || !isLikelyEvoReceiver(sel.X) || len(call.Args) != 1 {
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
		// ZYS-971's Decisions give "already ..." as a literal example
		// marker alongside "wrote ...", "would add ...", "nothing to
		// write" — any Summary opening on "already" names a checked
		// precondition, the same already-satisfied shape API-046 flags
		// for Skipped(evo.Reason(...)).
		stamped := lower != "" && (strings.HasPrefix(lower, "already") || containsAnyMarker(lower, summaryStampMarkers))
		if !stamped {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, summaryStampNarrationFinding(filename, pos, exprDottedName(sel.X), text))
		return true
	})
	return findings
}

func summaryStampNarrationFinding(filename string, pos token.Position, recv, text string) Finding {
	if recv == "" {
		recv = "task"
	}
	return Finding{
		RuleID:   "API-060",
		Severity: "warning",
		Message:  recv + ".Summary(" + strconv.Quote(text) + ") reads as mutation/dry-run/already-satisfied narration, not the caller's own result metadata",
		File:     filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Suggestion: "move this narration to the primitive that owns it — evo.File/evo.Effect's own record, ResolutionAlreadySatisfied (via " + recv +
			".Verify), or evo.Fact — and reserve " + recv + ".Summary for result metadata such as a count or verdict",
	}
}

// exportedReasonName turns a reason literal into an exported-style Go
// identifier fragment ("dirty worktree" -> "DirtyWorktree") for the var-name
// this rule's suggestion spells out.
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

// ===== API-051: a loop flattens structured findings into one joined error
// string, or creates one fake Task per finding and fails it, because the
// caller has no ergonomic way to retain many structured findings on one Task
// (zq hook_findings.go's blockStagedGolangciFindings ->
// errors.New(strings.Join(...)), hook.go's reportFileIntegrityIssues ->
// Task(file).Fail per finding — the two shapes ZYS-848's Contract names).
// TaskHandle.Problem (ZYS-848 Decisions 2026-09-23; docs/migration/1.1.md)
// lets one owning Task accumulate every finding as a structured Problem
// instead, so this rule cannot recommend its fix for a pin older than 1.1.0.

// detectPerFindingFakeTask flags `<expr>.Task(<x>).Fail(...)` /
// `.Failf(...)` inside a for/range loop body — a fake Task created only to
// display one finding, never independently schedulable or awaited.
func detectPerFindingFakeTask(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch s := n.(type) {
		case *ast.RangeStmt:
			body = s.Body
		case *ast.ForStmt:
			body = s.Body
		default:
			return true
		}
		if body == nil {
			return true
		}
		ast.Inspect(body, func(n2 ast.Node) bool {
			call, ok := n2.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Fail" && sel.Sel.Name != "Failf") {
				return true
			}
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			innerSel, ok := inner.Fun.(*ast.SelectorExpr)
			if !ok || innerSel.Sel.Name != "Task" {
				return true
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, Finding{
				RuleID:   "API-051",
				Severity: "error",
				Message:  "loop creates one Task per finding and immediately fails it; a Task should own its independent lifecycle, not stand in for one finding",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
				Suggestion: "replace the per-item .Task(...).Fail/Failf(...) with one owning Task that calls " +
					"task.Problem(summary, evo.Location(path, line, 0), evo.Code(code)) once per finding inside the loop, " +
					"then Define resolves the Task Failed once if any Problem was accumulated",
			})
			return true
		})
		return true
	})
	return findings
}

// flattenedDiagnosticsAppend matches `name = append(name, ...)` so the
// accumulator's own identifier is captured, not guessed — the loop body is
// already the smallest brace-balanced substring flattenedDiagnosticsLoops
// hands in, so this never crosses into an unrelated loop's accumulator.
var flattenedDiagnosticsAppend = regexp.MustCompile(`(\w+)\s*=\s*append\(\s*(\w+)\s*,`)

// flattenedDiagnosticsWrap reports whether rest (the function body's text
// after the accumulating loop) later wraps strings.Join(name, ...) directly
// inside errors.New(...), fmt.Errorf(...), or a .Fail/.Failf(...) call — the
// three shapes that discard every finding's own location/code/detail down
// to one flattened string.
func flattenedDiagnosticsWrap(rest, name string) bool {
	quoted := regexp.QuoteMeta(name)
	wrap := regexp.MustCompile(
		`(?:errors\.New|fmt\.Errorf|\.Failf?)\(\s*(?:"[^"]*",\s*)?strings\.Join\(\s*` + quoted + `\s*,`,
	)
	return wrap.MatchString(rest)
}

// detectFlattenedDiagnosticsLoop flags a for/range loop that appends into a
// slice, followed later in the same function by that slice joined straight
// into a single wrapped error/failure — the flattened-string shape ZYS-848's
// Contract calls out (zq's blockStagedGolangciFindings).
func detectFlattenedDiagnosticsLoop(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		for _, loop := range forLoopBodies(fn.body) {
			match := flattenedDiagnosticsAppend.FindStringSubmatch(loop.body)
			if match == nil || match[1] != match[2] {
				continue
			}
			name := match[1]
			rest := fn.body[loop.offset+len(loop.body):]
			if !flattenedDiagnosticsWrap(rest, name) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:   "API-051",
				Severity: "error",
				Message: "loop concatenates structured findings into " + name +
					", later flattened into one joined error string that discards each finding's own location/code/detail",
				File: filename,
				Line: lineAt(src, fn.offset+loop.offset),
				Suggestion: "accumulate each finding with task.Problem(summary, evo.Location(path, line, 0), evo.Code(code)) " +
					"inside the loop instead of building " + name + " for strings.Join/errors.New",
			})
		}
	}
	return findings
}

// loopBody is a for/range loop's brace-balanced body and its byte offset
// within the enclosing text — the same shape allFuncBodies uses for
// function bodies, scoped down to one loop.
type loopBody struct {
	body   string
	offset int
}

// forLoopBodies finds every top-level "for " loop's brace-balanced body in
// src, mirroring allFuncBodies' "func " scan.
func forLoopBodies(src string) []loopBody {
	var out []loopBody
	for i := 0; i < len(src); {
		idx := strings.Index(src[i:], "for ")
		if idx < 0 {
			break
		}
		idx += i
		if body, start, ok := balancedBraceBody(src, idx); ok {
			out = append(out, loopBody{body: body, offset: start})
			i = start + len(body)
		} else {
			i = idx + len("for ")
		}
	}
	return out
}

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

// ===== API-053: nested Evo resource acquisition (ZYS-933/ZYS-840). Effect
// holds spec.Resource for its fn callback's duration only when spec.Resource
// is set (File always claims its own path automatically, but takes no
// callback, so it cannot itself host a nested acquisition). Calling File or
// a Resource-claiming Effect again with that same held ctx — directly, or
// one call away through a same-file helper handed the held ctx — asks for a
// second resource while the first is still held. The runtime check
// (evo.ErrNestedResourceAcquisition) rejects it deterministically at apply
// time; this rule catches the same shape, including the indirect case,
// before it can reach runtime.

// resourceHold is one evo.Effect(ctx, spec, fn) call whose spec statically
// claims a Resource: fn's body and the name fn's own context.Context
// parameter binds to inside that body.
type resourceHold struct {
	body    *ast.BlockStmt
	ctxName string
}

// resourceHoldingCallbacks finds every evo.Effect(...) call in file whose
// spec composite literal (inline, or a same-file local variable last
// assigned one, resolved through specVars) sets a non-nil Resource field.
func resourceHoldingCallbacks(file *ast.File, pkg string, specVars map[string]*ast.CompositeLit) []resourceHold {
	var holds []resourceHold
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || calledFuncDotted(call) != pkg+".Effect" || len(call.Args) < 3 {
			return true
		}
		lit := effectSpecArg(call.Args[1], specVars)
		if lit == nil || !compositeLitClaimsResource(lit) {
			return true
		}
		fl, ok := call.Args[2].(*ast.FuncLit)
		if !ok || fl.Type.Params == nil || len(fl.Type.Params.List) == 0 {
			return true
		}
		names := fl.Type.Params.List[0].Names
		if len(names) == 0 {
			return true
		}
		holds = append(holds, resourceHold{body: fl.Body, ctxName: names[0].Name})
		return true
	})
	return holds
}

// effectSpecArg resolves arg (the second argument to evo.Effect) to its
// pkg.EffectSpec composite literal, whether written inline or through a
// same-file local variable resolved via specVars.
func effectSpecArg(arg ast.Expr, specVars map[string]*ast.CompositeLit) *ast.CompositeLit {
	if lit, ok := arg.(*ast.CompositeLit); ok {
		return lit
	}
	if id, ok := arg.(*ast.Ident); ok {
		return specVars[id.Name]
	}
	return nil
}

// effectSpecCompositeLits maps a local variable name to the pkg.EffectSpec
// composite literal it was last assigned from, same-file only — the same
// "obvious static case" scope every helper-following MCP rule in this file
// uses (no cross-package resolution, no type checking).
func effectSpecCompositeLits(file *ast.File, pkg string) map[string]*ast.CompositeLit {
	out := map[string]*ast.CompositeLit{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			lit, ok := rhs.(*ast.CompositeLit)
			if !ok {
				continue
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || exprDottedName(sel.X) != pkg || sel.Sel.Name != "EffectSpec" {
				continue
			}
			if id, ok := assign.Lhs[i].(*ast.Ident); ok {
				out[id.Name] = lit
			}
		}
		return true
	})
	return out
}

// compositeLitClaimsResource reports whether lit sets a Resource field to
// a non-nil expression.
func compositeLitClaimsResource(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Resource" {
			continue
		}
		id, ok := kv.Value.(*ast.Ident)
		return !ok || id.Name != "nil"
	}
	return false
}

// detectNestedResourceAcquisition walks every Resource-claiming Effect
// callback and, one call away through same-file helpers it hands its held
// ctx to, flags a second evo.File or Resource-claiming evo.Effect call made
// with that same held context identifier.
func detectNestedResourceAcquisition(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	funcs := map[string]*ast.FuncDecl{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
			funcs[fd.Name.Name] = fd
		}
		return true
	})
	specVars := effectSpecCompositeLits(file, pkg)

	var findings []Finding
	for _, hold := range resourceHoldingCallbacks(file, pkg, specVars) {
		visited := map[*ast.BlockStmt]bool{}
		var visit func(block *ast.BlockStmt, ctxName string, depth int)
		visit = func(block *ast.BlockStmt, ctxName string, depth int) {
			if block == nil || visited[block] || depth > 2 || ctxName == "" || ctxName == "_" {
				return
			}
			visited[block] = true
			findings = append(findings, scanBlockForNestedResourceAcquisition(filename, block, fset, pkg, ctxName, specVars)...)
			ast.Inspect(block, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				fd, ok := funcs[calledFuncName(call)]
				if !ok {
					return true
				}
				visit(fd.Body, forwardedParamName(fd, call, ctxName), depth+1)
				return true
			})
		}
		visit(hold.body, hold.ctxName, 0)
	}
	return findings
}

// scanBlockForNestedResourceAcquisition looks for pkg.File(ctxName, ...) or
// a Resource-claiming pkg.Effect(ctxName, ...) call anywhere in block,
// never descending into a nested FuncLit (that closure's own resource
// holds, if any, are tracked as their own resourceHold instead).
func scanBlockForNestedResourceAcquisition(filename string, block *ast.BlockStmt, fset *token.FileSet, pkg, ctxName string, specVars map[string]*ast.CompositeLit) []Finding {
	var findings []Finding
	ast.Inspect(block, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		dotted := calledFuncDotted(call)
		if dotted != pkg+".File" && dotted != pkg+".Effect" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		id, ok := call.Args[0].(*ast.Ident)
		if !ok || id.Name != ctxName {
			return true
		}
		if dotted == pkg+".Effect" {
			if len(call.Args) < 2 {
				return true
			}
			lit := effectSpecArg(call.Args[1], specVars)
			if lit == nil || !compositeLitClaimsResource(lit) {
				return true
			}
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, nestedResourceAcquisitionFinding(filename, pos, dotted))
		return true
	})
	return findings
}

// forwardedParamName reports the name fd's parameter list binds to at
// call's matching argument position for ctxName — the same same-file,
// name-based "one call away" resolution every helper-following MCP rule in
// this file uses (no cross-package resolution, no type checking).
func forwardedParamName(fd *ast.FuncDecl, call *ast.CallExpr, ctxName string) string {
	if fd.Type.Params == nil {
		return ""
	}
	var params []string
	for _, field := range fd.Type.Params.List {
		if len(field.Names) == 0 {
			params = append(params, "")
			continue
		}
		for _, n := range field.Names {
			params = append(params, n.Name)
		}
	}
	for i, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || id.Name != ctxName {
			continue
		}
		if i < len(params) {
			return params[i]
		}
	}
	return ""
}

func nestedResourceAcquisitionFinding(filename string, pos token.Position, calleeDotted string) Finding {
	return Finding{
		RuleID:     "API-053",
		Severity:   "error",
		Message:    calleeDotted + "(...) is called with a context that already holds a Resource from an enclosing evo.Effect's spec.Resource claim; holding at most one Resource at a time is what makes deadlock impossible, so a second acquisition is misuse even when the second resource is free",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "finish and return from the first evo.Effect/evo.File before starting a second, or claim one coarser Resource (e.g. evo.FSResource covering both paths) that both mutations share instead of nesting a second acquisition",
	}
}

// ===== API-054: generic bytes.Buffer/io.MultiWriter/task.Writer plumbing
// wired around a raw os/exec.Cmd solely to recreate Exec's own capture,
// liveness, and cancellation classification — or cancellation recognized by
// comparing captured output strings — when evo.Exec now returns an
// inspectable ExecResult (ZYS-850) that already owns exactly this. zq
// evidence: internal/app/run_captured_task.go allocates its own
// bytes.Buffer, combines task.Writer() with that buffer via io.MultiWriter,
// recognizes cancellation by comparing output strings, and classifies
// nonzero exit itself instead of inspecting ExecResult/ErrExecNonzeroExit.

// manualSubprocessCaptureCodeSignals are the code-shape tokens (identifiers/
// calls, never legitimate inside a string literal or comment) that, beside a
// function wiring a raw os/exec.Cmd's Stdout/Stderr to an Evo Task's own
// Writer(), corroborate the manual-recapture-of-Exec shape this rule
// targets: a hand-rolled buffer/multiwriter combine.
var manualSubprocessCaptureCodeSignals = []string{
	"bytes.Buffer", "bytes.NewBuffer", "MultiWriter(",
}

// manualSubprocessCaptureLiteralSignals are quoted signal text that, unlike
// manualSubprocessCaptureCodeSignals, is meant to be found as real string
// literal *content* in the code — the anti-pattern is comparing captured
// output against exactly this text (a cancellation check written as
// strings.Contains(out, "signal: killed") instead of an error/context
// check) — so, unlike the code-shape signals, these are matched against
// source with only comments masked, not string literals.
var manualSubprocessCaptureLiteralSignals = []string{
	`"signal: killed"`, `"signal: interrupt"`, `"context canceled"`,
}

// rawExecCmdSignals mark that the function drives a raw os/exec.Cmd (as
// opposed to some unrelated io.Writer plumbing) — required alongside
// task.Writer() so this rule only fires on code actually reimplementing
// Exec, not any bytes.Buffer/MultiWriter combination in the codebase. Like
// manualSubprocessCaptureCodeSignals, these are code-shape (an actual
// method call/type on a real exec.Cmd) and never legitimate as string
// literal or comment text, so they are matched with both masked out.
var rawExecCmdSignals = []string{"cmd.Run(", "cmd.Start(", "cmd.Output(", "cmd.CombinedOutput(", "exec.Cmd"}

func detectManualSubprocessCaptureAroundTask(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		funcStart := strings.LastIndex(src[:fn.offset], "func ")
		if funcStart < 0 {
			funcStart = fn.offset
		}
		wholeFunc := src[funcStart : fn.offset+len(fn.body)]
		// codeOnly blanks both comments and string literals so an
		// identifier-shaped signal (a real method call/type) only matches
		// when it is actually Go syntax, never text that merely mentions it
		// inside a log message, doc comment, or unrelated string literal.
		codeOnly := maskGoLexemes(wholeFunc, true)
		// literalsVisible blanks only comments, keeping string literal
		// content intact for signals that are meant to match real string
		// literal text in the source (the cancellation-string anti-pattern).
		literalsVisible := maskGoLexemes(wholeFunc, false)

		if !strings.Contains(codeOnly, ".Writer()") {
			continue
		}
		if !containsAnyToken(codeOnly, rawExecCmdSignals) {
			continue
		}
		signal, idx := firstContainedToken(codeOnly, manualSubprocessCaptureCodeSignals)
		if signal == "" {
			signal, idx = firstContainedToken(literalsVisible, manualSubprocessCaptureLiteralSignals)
		}
		if signal == "" {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   "API-054",
			Severity: "error",
			Message:  "a raw os/exec.Cmd wired to an Evo Task's Writer() reimplements Exec's own capture/liveness/cancellation with hand-rolled " + signal + " plumbing instead of inspecting the ExecResult evo.Exec already returns",
			File:     filename,
			Line:     lineAt(src, funcStart+idx),
			Suggestion: "replace the raw exec.Cmd, its manual bytes.Buffer/io.MultiWriter capture, and any output-string cancellation match with " +
				"res, err := evo.Exec(ctx, spec); inspect res (ExecResult: Ran/ExitCode/Stdout/Stderr/Truncated) and errors.Is(err, evo.ErrExecNonzeroExit) instead",
		})
	}
	return findings
}

// maskGoLexemes returns src with Go comments blanked out (replaced with
// spaces, newlines preserved) so token matching never fires on identifier-
// shaped text inside a comment. When alsoMaskStrings is true, string and
// rune literals are blanked too, for signals that must only match real Go
// syntax (an actual method call or type), never a mention of that text
// inside an unrelated string literal. The result has the same byte length
// and line breaks as src, so a byte offset found in the masked text is a
// valid offset into src for lineAt.
func maskGoLexemes(src string, alsoMaskStrings bool) string {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(file, []byte(src), nil, scanner.ScanComments)
	out := []byte(src)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT || (alsoMaskStrings && (tok == token.STRING || tok == token.CHAR)) {
			blankSpan(out, file.Offset(pos), len(lit))
		}
	}
	return string(out)
}

// blankSpan overwrites b[start:start+length] with spaces, leaving newlines
// untouched so line numbers computed from the result still match src.
func blankSpan(b []byte, start, length int) {
	for i := start; i < start+length && i >= 0 && i < len(b); i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

// containsAnyToken reports whether s contains any of the given substrings.
func containsAnyToken(s string, tokens []string) bool {
	for _, tok := range tokens {
		if strings.Contains(s, tok) {
			return true
		}
	}
	return false
}

// firstContainedToken returns the first token from tokens (in the given
// order) that occurs in s, and its byte offset within s — used to pick a
// stable, meaningful finding line among several corroborating signals.
func firstContainedToken(s string, tokens []string) (token string, idx int) {
	best := -1
	for _, tok := range tokens {
		if i := strings.Index(s, tok); i >= 0 && (best < 0 || i < best) {
			best = i
			token = tok
		}
	}
	return token, best
}

// ===== API-055: a caller-managed sync.Mutex/RWMutex Lock/Unlock wrapped
// around an evo.File call (ZYS-931). ZYS-840's automatic resource
// coordination already makes File claim its own path for writing with no
// caller code — the manual lock is redundant at best and, because it is
// invisible to Evo's own resource wait, can mask real contention or compete
// with it at worst. Scoped to sync.Mutex/RWMutex specifically (not every
// domain lock) so an unrelated in-memory guard never fires.

// lockCallPattern matches a bare .Lock()/.RLock() call so the receiver's
// dotted name can be recovered with identBefore.
var lockCallPattern = regexp.MustCompile(`\.(Lock|RLock)\(\)`)

func detectManualLockAroundEvoFile(filename, src string) []Finding {
	if !strings.Contains(src, "sync.Mutex") && !strings.Contains(src, "sync.RWMutex") {
		return nil
	}
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		body := fn.body
		if !strings.Contains(body, "evo.File(") && !strings.Contains(body, "evo.FileSpec{") {
			continue
		}
		loc := lockCallPattern.FindStringIndex(body)
		if loc == nil {
			continue
		}
		recv := identBefore(body, loc[0])
		if recv == "" {
			continue
		}
		lockMethod := "Lock()"
		unlockMethod := "Unlock()"
		if strings.HasPrefix(body[loc[0]:], ".RLock()") {
			lockMethod = "RLock()"
			unlockMethod = "RUnlock()"
		}
		rest := body[loc[1]:]
		deferUnlock := "defer " + recv + "." + unlockMethod
		bareUnlock := recv + "." + unlockMethod
		scopeEnd := len(body)
		if !strings.Contains(rest, deferUnlock) {
			if idx := strings.Index(rest, bareUnlock); idx >= 0 {
				scopeEnd = loc[1] + idx
			}
		}
		guarded := body[loc[0]:scopeEnd]
		if !strings.Contains(guarded, "evo.File(") && !strings.Contains(guarded, "evo.FileSpec{") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   "API-055",
			Severity: "error",
			Message:  recv + " manually locks/unlocks around an evo.File call — File already claims its own path for writing with no caller code",
			File:     filename,
			Line:     lineAt(src, fn.offset+loc[0]),
			Suggestion: "delete " + recv + "." + lockMethod + "/" + recv + "." + unlockMethod +
				" (and the sync.Mutex/RWMutex field) and call evo.File(ctx, evo.FileSpec{Path: ...}) directly;" +
				" for a non-File operation over the same path use evo.Effect(ctx, evo.EffectSpec{..., Resource: evo.FSResource(path)}, fn) instead of a caller lock",
		})
	}
	return findings
}
