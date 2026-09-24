// Package review — API-051: findings flattened into one error or faked as one Task each instead of accumulated with TaskHandle.Problem.
package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

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
				RuleID:  "API-051",
				Message: "loop creates one Task per finding and immediately fails it; a Task should own its independent lifecycle, not stand in for one finding",
				File:    filename,
				Line:    pos.Line,
				Column:  pos.Column,
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
				RuleID: "API-051",
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
