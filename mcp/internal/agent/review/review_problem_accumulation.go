// Package review — API-051: findings flattened into one error or faked as one Task each instead of accumulated with Task.Problem.
package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// ===== API-051: a loop flattens structured findings into one joined error
// string, or creates one fake Task per finding that only fails, because the
// caller has no ergonomic way to retain many structured findings on one Task
// (zq hook_findings.go's blockStagedGolangciFindings ->
// errors.New(strings.Join(...)), hook.go's reportFileIntegrityIssues -> one
// failing Task per finding). Task.Problem lets one owning Task accumulate
// every finding as a structured Problem instead, so this rule cannot
// recommend its fix for a pin older than 1.1.0.

// detectPerFindingFakeTask flags `<expr>.Task(<x>).Define(func... { return
// evo.Fail(...) })` inside a for/range loop body: a Task declared only to
// display one finding, whose whole callback is the failure, never
// independently schedulable work.
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
			if !ok || !isFailOnlyTaskDefine(call) {
				return true
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, Finding{
				RuleID:  "API-051",
				Message: "loop declares one Task per finding whose callback only returns evo.Fail; a Task should own real work, not stand in for one finding",
				File:    filename,
				Line:    pos.Line,
				Column:  pos.Column,
				Suggestion: "replace the per-item Task with one owning Task that calls " +
					"task.Problem(summary, evo.On(location), evo.Code(code)) once per finding inside its Define callback, " +
					"then returns evo.Fail once if any Problem was recorded",
			})
			return true
		})
		return true
	})
	return findings
}

// isFailOnlyTaskDefine reports whether call is <recv>.Task(x).Define(fn)
// where fn is a function literal whose body is the single statement
// `return <pkg>.Fail(...)`.
func isFailOnlyTaskDefine(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Define" || len(call.Args) < 1 {
		return false
	}
	if !isTaskChain(sel.X) {
		return false
	}
	fl, ok := call.Args[0].(*ast.FuncLit)
	if !ok || len(fl.Body.List) != 1 {
		return false
	}
	ret, ok := fl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	failCall, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	failSel, ok := failCall.Fun.(*ast.SelectorExpr)
	return ok && failSel.Sel.Name == "Fail"
}

// flattenedDiagnosticsAppend matches `name = append(name, ...)` so the
// accumulator's own identifier is captured, not guessed — the loop body is
// already the smallest brace-balanced substring flattenedDiagnosticsLoops
// hands in, so this never crosses into an unrelated loop's accumulator.
var flattenedDiagnosticsAppend = regexp.MustCompile(`(\w+)\s*=\s*append\(\s*(\w+)\s*,`)

// flattenedDiagnosticsWrap reports whether rest (the function body's text
// after the accumulating loop) later wraps strings.Join(name, ...) directly
// inside errors.New(...), fmt.Errorf(...), or an evo.Fail or evo.Refuse
// call: the shapes that discard every finding's own location/code/detail
// down to one flattened string.
func flattenedDiagnosticsWrap(rest, name string) bool {
	quoted := regexp.QuoteMeta(name)
	wrap := regexp.MustCompile(
		`(?:errors\.New|fmt\.Errorf|\.Fail|\.Refuse)\(\s*(?:"[^"]*",\s*)?strings\.Join\(\s*` + quoted + `\s*,`,
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
				RuleID: "API-051",
				Message: "loop concatenates structured findings into " + name +
					", later flattened into one joined error string that discards each finding's own location/code/detail",
				File: filename,
				Line: lineAt(src, fn.offset+loop.offset),
				Suggestion: "accumulate each finding with task.Problem(summary, evo.On(location), evo.Code(code)) " +
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
