// Package review — TAX-003, API-046, and API-060: taxonomy reasons and Summary text that restate lifecycle instead of stating a classification or result.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// ===== TAX-003: an evo.Reason("literal") used inline as a call argument in
// non-test source, and a reason that merely restates its verb.

// taxonomyReasonVerbWords maps the taxonomy verb to the word it must not be
// restated by its own reason (zq cmd/zq-build/main.go:81's
// Skipped(evo.Reason("skipped"))).
var taxonomyReasonVerbWords = map[string]string{"Skipped": "skipped"}

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
		if containsAny(lower, inapplicabilityReasonPhrases) {
			return true
		}
		if !containsAny(lower, alreadySatisfiedReasonPhrases) && !alreadySatisfiedReasonWords[lower] {
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
		RuleID:  "API-046",
		Message: "Skipped(evo.Reason(" + strconv.Quote(text) + ")) reports \"did not apply\"; the reason names a condition that was checked and already held, which is ResolutionAlreadySatisfied",
		File:    filename,
		Line:    pos.Line,
		Column:  pos.Column,
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
		stamped := lower != "" && (strings.HasPrefix(lower, "already") || containsAny(lower, summaryStampMarkers))
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
		RuleID:  "API-060",
		Message: recv + ".Summary(" + strconv.Quote(text) + ") reads as mutation/dry-run/already-satisfied narration, not the caller's own result metadata",
		File:    filename,
		Line:    pos.Line,
		Column:  pos.Column,
		Suggestion: "move this narration to the primitive that owns it — evo.File/evo.Effect's own record, ResolutionAlreadySatisfied (via " + recv +
			".Verify), or evo.Fact — and reserve " + recv + ".Summary for result metadata such as a count or verdict",
	}
}

// exportedReasonName turns a reason literal into an exported-style Go
// identifier fragment ("dirty worktree" -> "DirtyWorktree") for the var-name
// this rule's suggestion spells out.
