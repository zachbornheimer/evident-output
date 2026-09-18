// Package review — EVO-STAMP-001/002, EVO-FACT-001, EVO-EFFECT-001:
// Done used as a generic print, a repeated sibling Task label, a fake
// Task success for information, or a planned mutation narrated as Done.
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

const evoStampRequiredVersion = dialectOneZero

func isPrintfDone(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	lit, ok := stringLit(call.Args[0])
	return ok && strings.Contains(lit, "%")
}

func doneSummaryLiteral(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	return stringLit(call.Args[0])
}

func isEvoFileCall(call *ast.CallExpr, pkg string) bool {
	if pkg == "" {
		pkg = "evo"
	}
	return calledFuncDotted(call) == pkg+".File"
}

// hasPrecedingDefineOrFile reports whether recv (a named Task handle) had
// Define/a mutation verb, or the body called evo.File, at a position before
// donePos. Chained Task(...).Done(...) has no named handle and never
// qualifies — that expression itself is the stamp.
func hasPrecedingDefineOrFile(body *ast.BlockStmt, recv string, donePos token.Pos, disproven map[string]bool, pkg string) bool {
	if recv == "" {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || call.Pos() >= donePos {
			return true
		}
		if isEvoFileCall(call, pkg) {
			found = true
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "Define" && !mutationVerbNames[sel.Sel.Name] {
			return true
		}
		if !isLikelyEvoTaskReceiver(sel.X, disproven) {
			return true
		}
		if exprDottedName(sel.X) == recv {
			found = true
		}
		return true
	})
	return found
}

func stampFinding(ruleID, severity, message, suggestion, filename string, pos token.Position) Finding {
	return Finding{
		RuleID:          ruleID,
		Severity:        severity,
		Message:         message,
		File:            filename,
		Line:            pos.Line,
		Column:          pos.Column,
		Suggestion:      suggestion,
		RequiredVersion: evoStampRequiredVersion,
	}
}

func quoteLabel(s string) string { return `"` + s + `"` }

// detectDoneUsedAsGenericPrint is EVO-STAMP-001: Task.Done(format, args)
// with no preceding Define/File on that handle — Done used as printf.
func detectDoneUsedAsGenericPrint(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	pkg := evoImportName(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		findings = append(findings, stamp001InBody(filename, body, fset, disproven, pkg)...)
	})
	return findings
}

func stamp001InBody(filename string, body *ast.BlockStmt, fset *token.FileSet, disproven map[string]bool, pkg string) []Finding {
	var findings []Finding
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isPrintfDone(call) {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Done" || !isLikelyEvoTaskReceiver(sel.X, disproven) {
			return true
		}
		chained := isTaskCall(sel.X)
		recv := exprDottedName(sel.X)
		if !chained && hasPrecedingDefineOrFile(body, recv, call.Pos(), disproven, pkg) {
			return true
		}
		pos := fset.Position(call.Pos())
		recvHint := recv
		if recvHint == "" {
			recvHint = "task"
		}
		findings = append(findings, stampFinding(
			"EVO-STAMP-001", "warning",
			"Task.Done is used as a generic print (format-string prose) with no preceding Define/File",
			"call "+recvHint+".Define(...) or evo.File, then "+recvHint+".Done() with no format args; Done is a resolution, not printf",
			filename, pos,
		))
		return true
	})
	return findings
}

type taskLabelSite struct {
	name   string
	pos    token.Pos
	inLoop bool
}

func loopBodyRanges(body *ast.BlockStmt) [][2]token.Pos {
	var ranges [][2]token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.ForStmt:
			if s.Body != nil {
				ranges = append(ranges, [2]token.Pos{s.Body.Pos(), s.Body.End()})
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				ranges = append(ranges, [2]token.Pos{s.Body.Pos(), s.Body.End()})
			}
		}
		return true
	})
	return ranges
}

func posInRanges(pos token.Pos, ranges [][2]token.Pos) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}

func taskNameLiteral(n ast.Node, disproven map[string]bool) (string, token.Pos, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", 0, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Task" && sel.Sel.Name != "Item") || !isLikelyEvoTaskReceiver(sel.X, disproven) {
		return "", 0, false
	}
	name, ok := stringLit(call.Args[0])
	if !ok || name == "" {
		return "", 0, false
	}
	return name, call.Pos(), true
}

// detectDuplicateSiblingTaskLabels is EVO-STAMP-002: the same Task name
// literal appears twice in one function, or a Task name literal sits
// inside a for/range (same label every iteration).
func detectDuplicateSiblingTaskLabels(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		findings = append(findings, stamp002InBody(filename, body, fset, disproven)...)
	})
	return findings
}

func stamp002InBody(filename string, body *ast.BlockStmt, fset *token.FileSet, disproven map[string]bool) []Finding {
	loops := loopBodyRanges(body)
	var sites []taskLabelSite
	ast.Inspect(body, func(n ast.Node) bool {
		name, pos, ok := taskNameLiteral(n, disproven)
		if !ok {
			return true
		}
		sites = append(sites, taskLabelSite{name: name, pos: pos, inLoop: posInRanges(pos, loops)})
		return true
	})
	count := map[string]int{}
	first := map[string]token.Pos{}
	for _, s := range sites {
		count[s.name]++
		if _, ok := first[s.name]; !ok {
			first[s.name] = s.pos
		}
	}
	var findings []Finding
	for _, s := range sites {
		switch {
		case s.inLoop:
			pos := fset.Position(s.pos)
			findings = append(findings, stampFinding(
				"EVO-STAMP-002", "error",
				"Task("+quoteLabel(s.name)+") inside a loop reuses one sibling label every iteration",
				"use Group.Task(item) with the loop variable as the name, not Task("+quoteLabel(s.name)+")",
				filename, pos,
			))
		case count[s.name] > 1 && s.pos != first[s.name]:
			pos := fset.Position(s.pos)
			findings = append(findings, stampFinding(
				"EVO-STAMP-002", "error",
				"duplicate sibling Task label "+quoteLabel(s.name)+"; 1.0 records ErrDuplicateSiblingName instead of merging",
				"give each sibling a distinct name — one Group.Task per item, never Task("+quoteLabel(s.name)+") twice",
				filename, pos,
			))
		}
	}
	return findings
}

var factAsFakeSuccessPatterns = []string{
	"mapped to", "maps to", "loaded from", "discovered", "found at",
}

var plannedMutationThroughDonePatterns = []string{
	"would add", "would write", "would create", "would delete",
	"would remove", "would update", "proposal only", "dry-run only",
	"planned only",
}

func containsAnyFold(s string, needles []string) bool {
	lower := strings.ToLower(s)
	for _, n := range needles {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}

func eachDoneLiteral(body *ast.BlockStmt, disproven map[string]bool, visit func(call *ast.CallExpr, sel *ast.SelectorExpr, summary string)) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Done" || !isLikelyEvoTaskReceiver(sel.X, disproven) {
			return true
		}
		summary, ok := doneSummaryLiteral(call)
		if !ok {
			return true
		}
		visit(call, sel, summary)
		return true
	})
}

// detectInformationalDataAsFakeTaskSuccess is EVO-FACT-001: Done("mapped
// to...") (and close informational shapes) painting a checkmark for a Fact.
func detectInformationalDataAsFakeTaskSuccess(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		eachDoneLiteral(body, disproven, func(call *ast.CallExpr, sel *ast.SelectorExpr, summary string) {
			if !containsAnyFold(summary, factAsFakeSuccessPatterns) {
				return
			}
			pos := fset.Position(call.Pos())
			recv := exprDottedNameOrDefault(sel.X, "task")
			findings = append(findings, stampFinding(
				"EVO-FACT-001", "warning",
				"informational data is stamped as Task success; a Fact is not work",
				"replace "+recv+".Done(...) with "+recv+".Fact(\"mapped to\", value)",
				filename, pos,
			))
		})
	})
	return findings
}

// detectPlannedMutationNarratedThroughDone is EVO-EFFECT-001: Done("would
// add...") / Done("proposal only") narrating a planned mutation as success.
func detectPlannedMutationNarratedThroughDone(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		eachDoneLiteral(body, disproven, func(call *ast.CallExpr, sel *ast.SelectorExpr, summary string) {
			if !containsAnyFold(summary, plannedMutationThroughDonePatterns) {
				return
			}
			pos := fset.Position(call.Pos())
			recv := exprDottedNameOrDefault(sel.X, "task")
			findings = append(findings, stampFinding(
				"EVO-EFFECT-001", "warning",
				"planned mutation is narrated through Done; DryRun mutation verbs / Record already own [planned]",
				"replace "+recv+".Done(...) with a mutation verb under Config.DryRun, or "+recv+".Record(\"add\", n, object)",
				filename, pos,
			))
		})
	})
	return findings
}
