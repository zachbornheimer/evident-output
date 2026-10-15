// Package review — TAX-003: an Exclude reason that restates its verb instead of stating why.
package review

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// restatingExclusionReasons are the reason texts that only repeat that the
// Task was excluded (zq cmd/zq-build/main.go:81's Exclude("skipped")): the
// outcome already says so, and Excluded folds into one tally, so the reason is
// the only place left to say why.
var restatingExclusionReasons = []string{"excluded", "exclude", "skipped", "skip", "not run", "kept"}

func detectRestatedExclusionReason(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" || strings.HasSuffix(filename, "_test.go") {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isEvoIdent(sel.X, pkg) || sel.Sel.Name != "Exclude" {
			return true
		}
		text, ok := stringLit(call.Args[0])
		if !ok || !slices.Contains(restatingExclusionReasons, strings.ToLower(strings.TrimSpace(text))) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "TAX-003",
			Message:    "reason " + strconv.Quote(text) + " merely restates Exclude; a reason names why, not what",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "name the cause instead of the verb, e.g. return " + pkg + `.Exclude("protected branch") from Define`,
		})
		return true
	})
	return findings
}
