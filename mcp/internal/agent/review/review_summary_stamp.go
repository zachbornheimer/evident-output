// Package review — API-060: a Summary literal that narrates what Evo already
// records. Summary is the settled row's headline for the caller's own
// result (a count, a verdict). Narrating a mutation ("wrote config.json"), a
// dry run ("would add 3 refs"), a no-op ("nothing to write") or an
// already-held state ("already up to date") restates what evo.File,
// evo.Effect, the Satisfied outcome and the dry-run tense already derive.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// summaryStampMarkers are substrings (checked case-insensitive against the
// full Summary text) that only ever narrate a mutation that already
// happened, a dry-run hypothetical, a no-op, or an already-satisfied
// precondition.
var summaryStampMarkers = []string{
	"wrote ", "would add", "would write", "would create", "would delete",
	"would update", "would remove", "nothing to write", "nothing to do",
	"already up to date", "already up-to-date", "already exists",
	"already satisfied", "already installed", "already current",
}

// detectSummaryStampNarration is API-060.
func detectSummaryStampNarration(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" || strings.HasSuffix(filename, "_test.go") {
		return nil
	}
	recvs := newEvoHandleReceivers(file, pkg)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Summary" || len(call.Args) != 1 || !recvs.Holds(sel.X) {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil || !isStampNarration(text) {
			return true
		}
		findings = append(findings, summaryStampNarrationFinding(filename, fset.Position(call.Pos()), exprDottedName(sel.X), text))
		return true
	})
	return findings
}

// isStampNarration reports whether text reads as narration of a mutation,
// a dry run or an already-held state: any Summary opening on "already"
// names a checked precondition, the same shape API-046 flags in an
// Exclude reason.
func isStampNarration(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return lower != "" && (strings.HasPrefix(lower, "already") || containsAny(lower, summaryStampMarkers))
}

func summaryStampNarrationFinding(filename string, pos token.Position, recv, text string) Finding {
	if recv == "" {
		recv = "task"
	}
	return Finding{
		RuleID:  "API-060",
		Message: recv + ".Summary(" + strconv.Quote(text) + ") reads as mutation, dry-run or already-satisfied narration, not the caller's own result",
		File:    filename,
		Line:    pos.Line,
		Column:  pos.Column,
		Suggestion: "drop the narration: evo.File and evo.Effect record the mutation, a dry run is already shown in the planned tense, and a Verify that returns true settles Satisfied; " +
			"keep " + recv + ".Summary for a result such as a count or verdict, or record the detail with " + recv + ".Fact",
	}
}
