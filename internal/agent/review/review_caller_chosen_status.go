// Package review — EVO-UI-004 (C21-012): a print call that hand-picks a
// color or a status word. Evo's renderer chooses glyph and color from Task
// state and adapts them to Plain, NoColor, TTY and GlyphProfile; a raw ANSI
// escape or a "[OK]"-style tag survives none of those. Only string literals
// printed directly are matched, so a color a library returns never fires.
package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// ansiEscapeIntroducer starts every ANSI color or cursor sequence.
const ansiEscapeIntroducer = "\x1b["

// statusTag is a bracketed status word a caller printed instead of
// resolving a Task.
var statusTag = regexp.MustCompile(`(?i)\[(OK|PASS(ED)?|FAIL(ED)?|ERROR|WARN(ING)?|DONE)\]`)

// detectCallerChosenGlyphColorOrStatus is EVO-UI-004.
func detectCallerChosenGlyphColorOrStatus(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isPrintCall(call) {
			return true
		}
		if what := chosenPresentation(call); what != "" {
			pos := fset.Position(call.Pos())
			findings = append(findings, Finding{
				RuleID:     "EVO-UI-004",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Message:    "caller-chosen " + what + " printed directly: evo's renderer owns glyph, color and status wording and adapts them to Plain, NoColor and TTY",
				Suggestion: "delete the hand-picked text; resolve the work through Define (or Fail/Block/Warn) and let the renderer choose glyph and color",
			})
		}
		return true
	})
	return findings
}

// chosenPresentation names the first hand-picked presentation among a
// print call's string-literal arguments: "color", "status", or "".
func chosenPresentation(call *ast.CallExpr) string {
	what := ""
	for _, arg := range call.Args {
		ast.Inspect(arg, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || what != "" {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			what = presentationOf(text)
			return true
		})
	}
	return what
}

func presentationOf(text string) string {
	switch {
	case strings.Contains(text, ansiEscapeIntroducer):
		return "color"
	case statusTag.MatchString(text):
		return "status"
	}
	return ""
}
