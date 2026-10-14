// Package review — EVO-UI-001/002/003 and EVO-WIRE-003: presentation the
// caller hand-prints when Evo already models the same information, and human
// text written to the stream that carries the data payload.
package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// printedLine is one hand-printed line: the call that prints it and the
// literal format or message it prints.
type printedLine struct {
	call    *ast.CallExpr
	literal string
	// toRunText is true when the line goes to Run.Text(), the writer for
	// free human text.
	toRunText bool
}

// printedLineOf reads n as fmt.Print/Printf/Println (stdout) or
// fmt.Fprint/Fprintf/Fprintln to a Run.Text() writer. It reports false for
// any other call, and for a message that is not a compile-time literal (a
// computed value cannot be pattern-matched honestly).
func printedLineOf(n ast.Node) (printedLine, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return printedLine{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isIdentNamed(sel.X, "fmt") {
		return printedLine{}, false
	}
	switch sel.Sel.Name {
	case "Print", "Printf", "Println":
		return literalPrinted(call, 0, false)
	case "Fprint", "Fprintf", "Fprintln":
		if len(call.Args) > 0 && isRunTextWriter(call.Args[0]) {
			return literalPrinted(call, 1, true)
		}
	}
	return printedLine{}, false
}

func literalPrinted(call *ast.CallExpr, msgArg int, toRunText bool) (printedLine, bool) {
	if len(call.Args) <= msgArg {
		return printedLine{}, false
	}
	literal, ok := printLiteral(call.Args[msgArg])
	return printedLine{call: call, literal: literal, toRunText: toRunText}, ok
}

// isRunTextWriter reports whether e is a call of a method named Text with
// no arguments, Run.Text()'s shape.
func isRunTextWriter(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Text"
}

func isIdentNamed(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// printLiteral returns the string literal arg is, unwrapping one level of
// fmt.Sprintf(...).
func printLiteral(arg ast.Expr) (string, bool) {
	if inner, ok := arg.(*ast.CallExpr); ok {
		if sel, ok := inner.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" && isIdentNamed(sel.X, "fmt") && len(inner.Args) > 0 {
			arg = inner.Args[0]
		}
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// forEachPrintedLine calls visit for every hand-printed line in f.
func forEachPrintedLine(f *ast.File, visit func(printedLine)) {
	ast.Inspect(f, func(n ast.Node) bool {
		if line, ok := printedLineOf(n); ok {
			visit(line)
		}
		return true
	})
}

func printedLineFinding(filename string, fset *token.FileSet, line printedLine, ruleID, message, suggestion string) Finding {
	pos := fset.Position(line.call.Pos())
	return Finding{RuleID: ruleID, Message: message, File: filename, Line: pos.Line, Column: pos.Column, Suggestion: suggestion}
}

// factLineLiteralPattern matches a "Label: %verb" literal — the shape of a
// routine key/value observation hand-printed instead of recorded as a Fact
// (EVO-UI-001).
var factLineLiteralPattern = regexp.MustCompile(`^([A-Za-z][\w ./-]{0,60}):\s*%[a-zA-Z]`)

// detectFactPrintedAsUIText is EVO-UI-001: a "label: value" line written to
// Run.Text() duplicates Fact, which the renderer and the JSON projection
// both derive from.
func detectFactPrintedAsUIText(filename string, f *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachPrintedLine(f, func(line printedLine) {
		m := factLineLiteralPattern.FindStringSubmatch(line.literal)
		if !line.toRunText || m == nil {
			return
		}
		findings = append(findings, printedLineFinding(filename, fset, line, "EVO-UI-001",
			`manually printed "`+m[1]+`: ..." line duplicates task.Fact; a Fact is derived and projected consistently across renderer and JSON`,
			`replace with task.Fact("`+m[1]+`", value)`))
	})
	return findings
}

// isSuccessConfirmationLine reports whether literal reads as a hand-printed
// completion confirmation — the checkmark glyph, or "verified"/"passed" as
// the line's leading or trailing word — rather than ordinary text that
// merely contains one of those words in passing, e.g. "license verified
// against upstream, expires in 30 days".
func isSuccessConfirmationLine(literal string) bool {
	if strings.Contains(literal, "✓") {
		return true
	}
	words := strings.Fields(strings.TrimRight(strings.TrimSpace(literal), ".!"))
	if len(words) == 0 {
		return false
	}
	first := strings.ToLower(words[0])
	last := strings.ToLower(words[len(words)-1])
	return first == "verified" || first == "passed" || last == "verified" || last == "passed"
}

// detectPassingVerificationPrinted is EVO-UI-002: a hand-printed success
// line duplicates the success row Evo already renders, and drifts from it
// under Plain, JSON and verbosity modes.
func detectPassingVerificationPrinted(filename string, f *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachPrintedLine(f, func(line printedLine) {
		if !isSuccessConfirmationLine(line.literal) {
			return
		}
		findings = append(findings, printedLineFinding(filename, fset, line, "EVO-UI-002",
			"manually printed success/verified line duplicates the success row the Task already renders when its Define callback returns nil",
			"delete the manual success line; a Task whose Define returns nil already shows as succeeded"))
	})
	return findings
}

// uiHandBuiltProgressPattern matches a literal's "N/M" or "N of M" shape.
var uiHandBuiltProgressPattern = regexp.MustCompile(`%d\s*(?:/|of)\s*%d`)

// uiProgressKeywordPattern matches a completion word that anchors the
// "%d/%d" shape to an actual progress count, distinguishing it from an
// unrelated "N/M"-shaped value: a ratio, a score, or one pair of a longer
// date/tuple format (e.g. "%d/%d/%d" for day/month/year, which also
// contains a bare "%d/%d" substring).
var uiProgressKeywordPattern = regexp.MustCompile(`(?i)\b(done|complete(?:d)?|remaining|finished|progress|processed|copied|copying|uploading|downloading|files?|tasks?|items?|steps?|records?)\b`)

// isHandBuiltProgressLine reports whether literal reads as a hand-assembled
// "N/M done" progress line: exactly two %d verbs in the N/M shape, plus a
// completion word.
func isHandBuiltProgressLine(literal string) bool {
	return uiHandBuiltProgressPattern.MatchString(literal) &&
		strings.Count(literal, "%d") == 2 &&
		uiProgressKeywordPattern.MatchString(literal)
}

// detectHandBuiltProgressText is EVO-UI-003: hand-assembled "N/M done" text
// duplicates counts Evo already derives from Task, Group and Sequence state
// and can silently disagree with them.
func detectHandBuiltProgressText(filename string, f *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachPrintedLine(f, func(line printedLine) {
		if !isHandBuiltProgressLine(line.literal) {
			return
		}
		findings = append(findings, printedLineFinding(filename, fset, line, "EVO-UI-003",
			"hand-built \"N/M\" progress text duplicates counts Evo already derives from Task and Group state",
			"replace with task.Progress(completed, total) or the owning Group/Sequence summary"))
	})
	return findings
}

// jsonPayloadToStdoutPattern matches a JSON/JSONL encoder writing to stdout:
// os.Stdout itself, or Run.ResultWriter(), which is stdout in Human and
// Plain output. jsonStdoutHumanTextPattern matches an ordinary human print
// reaching stdout in the same file (EVO-WIRE-003).
var (
	jsonPayloadToStdoutPattern = regexp.MustCompile(`json\.NewEncoder\(\s*(?:os\.Stdout|[\w.]+\.ResultWriter\(\))\s*\)\.Encode\(`)
	jsonStdoutHumanTextPattern = regexp.MustCompile(`\bfmt\.(?:Print|Printf|Println)\(|fmt\.Fprint(?:f|ln)?\(\s*os\.Stdout\s*,`)
)

// detectJSONStdoutMixedWithHumanText flags a file that encodes JSON/JSONL
// to stdout and also writes ordinary human text to stdout — a JSON/JSONL
// consumer cannot parse a stream with a human line spliced into it.
func detectJSONStdoutMixedWithHumanText(filename, src string) []Finding {
	if !jsonPayloadToStdoutPattern.MatchString(src) {
		return nil
	}
	loc := jsonStdoutHumanTextPattern.FindStringIndex(src)
	if loc == nil {
		return nil
	}
	return []Finding{{
		RuleID:     "EVO-WIRE-003",
		Message:    "human text written to stdout in a file that also encodes JSON/JSONL to stdout; a machine consumer cannot parse the mixed stream",
		File:       filename,
		Line:       lineAt(src, loc[0]),
		Suggestion: "write human text with fmt.Fprintln(r.Text(), ...), which goes to stderr, and the data payload through r.ResultWriter(), so stdout carries only the payload",
	}}
}
