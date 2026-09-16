package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// isFmtOrEvoPrintReceiver reports whether a Print/Printf/Println call's
// receiver is either the fmt package or an evo Task/Output presentation
// handle — the two surfaces EVO-UI-00x cares about. A receiver-agnostic
// match would also fire on log.Printf, a cobra *cobra.Command's
// cmd.Println, an http.ResponseWriter, or any other unrelated type that
// happens to expose a like-named method.
func isFmtOrEvoPrintReceiver(x ast.Expr) bool {
	if id, ok := x.(*ast.Ident); ok && id.Name == "fmt" {
		return true
	}
	return isLikelyEvoReceiver(x)
}

// isPrintFamilyMethod reports whether name is one of the plain (non-Fprint)
// Print-family method names EVO-UI-002/003 care about.
func isPrintFamilyMethod(name string) bool {
	switch name {
	case "Print", "Printf", "Println":
		return true
	default:
		return false
	}
}

// printLiteralArg returns the literal format/message string passed as a
// Print/Printf/Println call's first argument, unwrapping one level of
// fmt.Sprintf(...) nesting. It reports false when the first argument is not
// a compile-time string literal (a computed value can't be pattern-matched
// honestly).
func printLiteralArg(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	arg := call.Args[0]
	if inner, ok := arg.(*ast.CallExpr); ok {
		if sel, ok := inner.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "fmt" && len(inner.Args) > 0 {
				arg = inner.Args[0]
			}
		}
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// factLineLiteralPattern matches a "Label: %verb" literal — the shape of a
// routine key/value observation hand-printed instead of recorded as a Fact
// (EVO-UI-001).
var factLineLiteralPattern = regexp.MustCompile(`^([A-Za-z][\w ./-]{0,60}):\s*%[a-zA-Z]`)

// detectFactPrintedAsUIText flags a manually printed "label: value" line on
// an evo Task/Output handle — the renderer/JSON both already derive from
// Facts, and a hand-printed line is unstructured text neither can rely on.
func detectFactPrintedAsUIText(fset *token.FileSet, f *ast.File, filename string) []Finding {
	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Printf" || !isLikelyEvoReceiver(sel.X) {
			return true
		}
		literal, ok := printLiteralArg(call)
		if !ok {
			return true
		}
		m := factLineLiteralPattern.FindStringSubmatch(literal)
		if m == nil {
			return true
		}
		label := m[1]
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:     "EVO-UI-001",
			Severity:   "warning",
			Message:    `manually printed "` + label + `: ..." line duplicates task.Fact; a Fact is derived and projected consistently across renderer and JSON`,
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: `replace with task.Fact("` + label + `", value)`,
		})
		return true
	})
	return findings
}

// isSuccessConfirmationLine reports whether literal reads as a hand-printed
// completion confirmation — the checkmark glyph, or "verified"/"passed" as
// the line's leading or trailing word (EVO-UI-002) — rather than ordinary
// text that merely contains one of those words in passing, e.g. "license
// verified against upstream, expires in 30 days" or "2 hours passed since
// the last run".
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

// detectPassingVerificationPrinted flags a hand-printed success/verified
// line on fmt or an evo Task/Output handle, which duplicates the glyph
// Task.Done already renders on the passing path and drifts from it under
// Plain/JSON/verbosity modes.
func detectPassingVerificationPrinted(fset *token.FileSet, f *ast.File, filename string) []Finding {
	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isPrintFamilyMethod(sel.Sel.Name) || !isFmtOrEvoPrintReceiver(sel.X) {
			return true
		}
		literal, ok := printLiteralArg(call)
		if !ok || !isSuccessConfirmationLine(literal) {
			return true
		}
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:     "EVO-UI-002",
			Severity:   "warning",
			Message:    "manually printed success/verified line duplicates the glyph task.Done already renders on the passing path",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "delete the manual success line; let task.Done() render the passing state",
		})
		return true
	})
	return findings
}

// uiHandBuiltProgressPattern matches a literal that hand-assembles an
// "N/M" or "N of M" progress count (EVO-UI-003) instead of letting
// Task.Progress derive it from real state.
var uiHandBuiltProgressPattern = regexp.MustCompile(`%d\s*(?:/|of)\s*%d`)

// detectHandBuiltProgressText flags hand-assembled "N/M done" text on fmt
// or an evo Task/Output handle, which duplicates counts evo already
// derives from Task/Group/Sequence state and can silently disagree with
// them.
func detectHandBuiltProgressText(fset *token.FileSet, f *ast.File, filename string) []Finding {
	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isPrintFamilyMethod(sel.Sel.Name) || !isFmtOrEvoPrintReceiver(sel.X) {
			return true
		}
		literal, ok := printLiteralArg(call)
		if !ok || !uiHandBuiltProgressPattern.MatchString(literal) {
			return true
		}
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:     "EVO-UI-003",
			Severity:   "warning",
			Message:    "hand-built \"N/M\" progress text duplicates counts evo already derives from Task/Group state",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace with task.Progress(completed, total) or the owning Group/Sequence summary",
		})
		return true
	})
	return findings
}

// detectMarshalOfInternalSnapshot flags json.Marshal(x.Snapshot()) (or
// MarshalIndent), where x is an evo Task/Output/Group/Sequence handle —
// this bypasses the sanctioned, versioned JSON encoder and leaks
// undocumented internal field names/shape to consumers. evo has no
// .Result() accessor (Run/Output.Run return a Result value directly), so
// only .Snapshot() is a real evo misuse shape; the receiver check keeps
// this from firing on an unrelated type's own Snapshot() method.
func detectMarshalOfInternalSnapshot(fset *token.FileSet, f *ast.File, filename string) []Finding {
	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "json" || len(call.Args) == 0 {
			return true
		}
		fn := sel.Sel.Name
		if fn != "Marshal" && fn != "MarshalIndent" {
			return true
		}
		inner, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		innerSel, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok || innerSel.Sel.Name != "Snapshot" || !isLikelyEvoReceiver(innerSel.X) {
			return true
		}
		recv := exprDottedName(innerSel.X)
		snapshotCall := recv + ".Snapshot()"
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:     "EVO-WIRE-001",
			Severity:   "error",
			Message:    "json." + fn + " marshals the internal Snapshot directly; use the sanctioned JSON encoder instead",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace json." + fn + "(" + snapshotCall + ") with render.EncodeJSON(" + snapshotCall + ")",
		})
		return true
	})
	return findings
}

// jsonEncoderToStdoutPattern matches the canonical JSON/JSONL-to-stdout
// idiom; jsonStdoutHumanTextPattern matches an ordinary human print
// reaching stdout in the same file (EVO-WIRE-003).
var (
	jsonEncoderToStdoutPattern = regexp.MustCompile(`json\.NewEncoder\(\s*os\.Stdout\s*\)\.Encode\(`)
	jsonStdoutHumanTextPattern = regexp.MustCompile(`\bfmt\.(?:Print|Printf|Println)\(|fmt\.Fprint(?:f|ln)?\(\s*os\.Stdout\s*,`)
)

// detectJSONStdoutMixedWithHumanText flags a file that encodes JSON/JSONL
// to stdout and also writes ordinary human text to stdout — a JSON/JSONL
// consumer cannot parse a stream with a human line spliced into it.
func detectJSONStdoutMixedWithHumanText(filename, src string) []Finding {
	if !jsonEncoderToStdoutPattern.MatchString(src) {
		return nil
	}
	loc := jsonStdoutHumanTextPattern.FindStringIndex(src)
	if loc == nil {
		return nil
	}
	return []Finding{{
		RuleID:     "EVO-WIRE-003",
		Severity:   "error",
		Message:    "human text written to stdout in a file that also encodes JSON/JSONL to stdout; a machine consumer cannot parse the mixed stream",
		File:       filename,
		Line:       lineAt(src, loc[0]),
		Suggestion: "route human presentation to Stderr (evo.FormatData) so stdout carries only the JSON/JSONL payload",
	}}
}
