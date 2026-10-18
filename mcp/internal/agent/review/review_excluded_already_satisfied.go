// Package review — API-046: evo.Exclude("...") whose reason names a
// condition that was checked and already held. Excluded means policy chose
// not to run the Task; the Satisfied outcome — produced by a Verify
// precondition, or derived from evo.File and evo.Exec's own tracked
// comparison — means the state was already true. Collapsing the two into
// Excluded hides a real, checked precondition behind the "not run" word.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

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

// inapplicabilityReasonPhrases are substrings that name a precondition
// object that does not exist; they never fire API-046 even if they also
// loosely match "current" or "up to date" phrasing elsewhere in the string.
var inapplicabilityReasonPhrases = []string{
	"no project config", "no go module", "no go.mod", "not applicable",
	"n/a", "not a git repo", "not a repository", "no config found",
	"missing config", "no module found", "not present",
}

// detectExcludedForAlreadySatisfied is API-046.
func detectExcludedForAlreadySatisfied(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 1 || calledFuncDotted(call) != pkg+".Exclude" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil || !namesAlreadySatisfiedState(text) {
			return true
		}
		findings = append(findings, excludedAlreadySatisfiedFinding(filename, fset.Position(call.Pos()), pkg, text))
		return true
	})
	return findings
}

// namesAlreadySatisfiedState reports whether reason says a checked state
// already held, rather than naming a policy or a missing precondition.
func namesAlreadySatisfiedState(reason string) bool {
	lower := strings.ToLower(strings.TrimSpace(reason))
	if containsAny(lower, inapplicabilityReasonPhrases) {
		return false
	}
	return containsAny(lower, alreadySatisfiedReasonPhrases) || alreadySatisfiedReasonWords[lower]
}

func excludedAlreadySatisfiedFinding(filename string, pos token.Position, pkg, text string) Finding {
	return Finding{
		RuleID:  "API-046",
		Message: pkg + ".Exclude(" + strconv.Quote(text) + ") reports \"policy chose not to run it\"; the reason names a condition that was checked and already held, which is the Satisfied outcome",
		File:    filename,
		Line:    pos.Line,
		Column:  pos.Column,
		Suggestion: "add a precondition with task.Verify(func(ctx context.Context) (bool, error) { ... }) before task.Define(...) so Evo settles Satisfied on its own, " +
			"or let " + pkg + ".File/" + pkg + ".Exec derive it from their own tracked comparison; keep " + pkg + ".Exclude for a policy choice not to run the Task",
	}
}
