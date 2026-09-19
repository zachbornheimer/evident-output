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

// uiHandBuiltProgressPattern matches a literal's "N/M" or "N of M" shape.
var uiHandBuiltProgressPattern = regexp.MustCompile(`%d\s*(?:/|of)\s*%d`)

// uiProgressKeywordPattern matches a completion word that anchors the
// "%d/%d" shape to an actual progress count, distinguishing it from an
// unrelated "N/M"-shaped value: a ratio, a score, or one pair of a longer
// date/tuple format (e.g. "%d/%d/%d" for day/month/year, which also
// contains a bare "%d/%d" substring).
var uiProgressKeywordPattern = regexp.MustCompile(`(?i)\b(done|complete(?:d)?|remaining|finished|progress|processed|copied|copying|uploading|downloading|files?|tasks?|items?|steps?|records?)\b`)

// uiLiteralFractionPattern matches a concrete "14/40" fraction.
// uiLiteralDatePattern rejects day/month/year. uiNotProgressLabelPattern
// rejects a labeled score/ratio/date so those stay silent.
var (
	uiLiteralFractionPattern  = regexp.MustCompile(`\b\d{1,4}/\d{1,4}\b`)
	uiLiteralDatePattern      = regexp.MustCompile(`\b\d{1,4}/\d{1,4}/\d{1,4}\b`)
	uiNotProgressLabelPattern = regexp.MustCompile(`(?i)\b(score|ratio|average|avg|date)\b`)
)

// isHandBuiltProgressLine reports whether literal reads as a hand-assembled
// progress fraction: "14/40", a bare "%d/%d", or "%d/%d done" — not a
// labeled score/ratio or a three-part date.
func isHandBuiltProgressLine(literal string) bool {
	if uiNotProgressLabelPattern.MatchString(literal) || uiLiteralDatePattern.MatchString(literal) {
		return false
	}
	if uiHandBuiltProgressPattern.MatchString(literal) {
		n := strings.Count(literal, "%d")
		if n == 2 && uiProgressKeywordPattern.MatchString(literal) {
			return true
		}
		trimmed := strings.TrimSpace(strings.ReplaceAll(literal, "\n", ""))
		if n == 2 && (trimmed == "%d/%d" || trimmed == "%d of %d") {
			return true
		}
	}
	return uiLiteralFractionPattern.MatchString(literal)
}

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
		if !ok || !isHandBuiltProgressLine(literal) {
			return true
		}
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:          "EVO-UI-003",
			Severity:        "warning",
			Message:         "hand-built \"N/M\" progress text duplicates counts evo already derives from Task/Group state",
			File:            filename,
			Line:            pos.Line,
			Column:          pos.Column,
			Suggestion:      "replace with task.Progress(completed, total) or the owning Group/Sequence summary",
			RequiredVersion: dialectOneZero,
		})
		return true
	})
	return findings
}

// detectMarshalOfInternalSnapshot flags encoding/json marshaling of evo
// Snapshot / internal engine types (json.Marshal(out.Snapshot()),
// json.Marshal(snap) after snap := out.Snapshot(), json.NewEncoder.Encode
// of the same, or json.Marshal of an engine/core composite). That bypasses
// the sanctioned, versioned encoder and leaks undocumented field layout.
func detectMarshalOfInternalSnapshot(fset *token.FileSet, f *ast.File, filename string) []Finding {
	snapshotVars := collectSnapshotVars(f)
	engineAliases := engineImportAliases(f)
	engineVars := collectEngineVars(f, engineAliases)
	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		arg, fn, ok := jsonEncodeArg(call)
		if !ok || !isInternalRuntimeJSONArg(arg, snapshotVars, engineVars, engineAliases) {
			return true
		}
		argText := exprDottedNameOrDefault(arg, "snapshot")
		if isSnapshotCall(arg) {
			recv := ""
			if sel, ok := arg.(*ast.CallExpr); ok {
				if s, ok := sel.Fun.(*ast.SelectorExpr); ok {
					recv = exprDottedName(s.X)
				}
			}
			if recv != "" {
				argText = recv + ".Snapshot()"
			}
		}
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:          "EVO-WIRE-001",
			Severity:        "error",
			Message:         "json." + fn + " marshals internal Snapshot/runtime state as if it were public API; use the sanctioned JSON encoder instead",
			File:            filename,
			Line:            pos.Line,
			Column:          pos.Column,
			Suggestion:      "replace json." + fn + "(" + argText + ") with render.EncodeJSON(" + argText + ")",
			RequiredVersion: dialectOneZero,
		})
		return true
	})
	return findings
}

func collectSnapshotVars(file *ast.File) map[string]bool {
	vars := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if i >= len(assign.Lhs) {
				continue
			}
			id, ok := assign.Lhs[i].(*ast.Ident)
			if !ok {
				continue
			}
			if isSnapshotCall(rhs) {
				vars[id.Name] = true
			}
		}
		return true
	})
	return vars
}

func isSnapshotCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Snapshot" && isLikelyEvoReceiver(sel.X)
}

func engineImportAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !strings.Contains(path, "evident-output/internal/engine") && !strings.Contains(path, "evident-output/internal/core") {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "." && imp.Name.Name != "_" {
			aliases[imp.Name.Name] = true
			continue
		}
		if strings.HasSuffix(path, "/engine") {
			aliases["engine"] = true
		} else {
			aliases["core"] = true
		}
	}
	return aliases
}

func isEngineTypeExpr(e ast.Expr, aliases map[string]bool) bool {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return isEngineTypeExpr(v.Type, aliases)
	case *ast.UnaryExpr:
		return isEngineTypeExpr(v.X, aliases)
	case *ast.StarExpr:
		return isEngineTypeExpr(v.X, aliases)
	case *ast.CallExpr:
		return isEngineTypeExpr(v.Fun, aliases)
	case *ast.SelectorExpr:
		id, ok := v.X.(*ast.Ident)
		return ok && aliases[id.Name]
	default:
		return false
	}
}

func collectEngineVars(file *ast.File, aliases map[string]bool) map[string]bool {
	vars := map[string]bool{}
	if len(aliases) == 0 {
		return vars
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			if v.Type == nil || v.Type.Params == nil {
				return true
			}
			for _, field := range v.Type.Params.List {
				if !isEngineTypeExpr(field.Type, aliases) {
					continue
				}
				for _, name := range field.Names {
					vars[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range v.Rhs {
				if i >= len(v.Lhs) {
					continue
				}
				id, ok := v.Lhs[i].(*ast.Ident)
				if !ok {
					continue
				}
				if isEngineTypeExpr(rhs, aliases) {
					vars[id.Name] = true
				}
			}
		case *ast.ValueSpec:
			if isEngineTypeExpr(v.Type, aliases) {
				for _, name := range v.Names {
					vars[name.Name] = true
				}
			}
		}
		return true
	})
	return vars
}

func isInternalRuntimeJSONArg(arg ast.Expr, snapshotVars, engineVars, engineAliases map[string]bool) bool {
	if isSnapshotCall(arg) {
		return true
	}
	if id, ok := arg.(*ast.Ident); ok && (snapshotVars[id.Name] || engineVars[id.Name]) {
		return true
	}
	return isEngineTypeExpr(arg, engineAliases)
}

func jsonEncodeArg(call *ast.CallExpr) (arg ast.Expr, fn string, ok bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return nil, "", false
	}
	if id, ok := sel.X.(*ast.Ident); ok && id.Name == "json" {
		switch sel.Sel.Name {
		case "Marshal", "MarshalIndent":
			return call.Args[0], sel.Sel.Name, true
		}
	}
	if sel.Sel.Name != "Encode" {
		return nil, "", false
	}
	inner, ok := sel.X.(*ast.CallExpr)
	if !ok {
		return nil, "", false
	}
	innerSel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || innerSel.Sel.Name != "NewEncoder" {
		return nil, "", false
	}
	pkg, ok := innerSel.X.(*ast.Ident)
	if !ok || pkg.Name != "json" {
		return nil, "", false
	}
	return call.Args[0], "NewEncoder.Encode", true
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

// uiCallerStatusTokenPattern matches a caller-invented status token such as
// [OK]/[FAIL] that evo's renderer already chooses from Task state.
var uiCallerStatusTokenPattern = regexp.MustCompile(`\[(?:OK|FAIL(?:ED)?|ERROR|WARN(?:ING)?|PASS(?:ED)?|DONE|SUCCESS)\]`)

// isCallerChosenStatusFormat reports whether literal is a hand-picked glyph,
// ANSI color, or bracketed status word (EVO-UI-004).
func isCallerChosenStatusFormat(literal string) bool {
	if strings.ContainsRune(literal, '\x1b') || strings.Contains(literal, `\x1b[`) || strings.Contains(literal, `\033[`) {
		return true
	}
	return uiCallerStatusTokenPattern.MatchString(literal)
}

// detectCallerChosenGlyphColor flags fmt/evo Print-family calls whose
// literal chooses glyph, color, or a free-form status token that evo's
// renderer already derives from Task state (EVO-UI-004).
func detectCallerChosenGlyphColor(fset *token.FileSet, f *ast.File, filename string) []Finding {
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
		if !ok || !isCallerChosenStatusFormat(literal) {
			return true
		}
		pos := fset.Position(n.Pos())
		findings = append(findings, Finding{
			RuleID:          "EVO-UI-004",
			Severity:        "warning",
			Message:         "caller chooses glyph/color/status formatting that evo's renderer already derives from Task state",
			File:            filename,
			Line:            pos.Line,
			Column:          pos.Column,
			Suggestion:      "delete the hand-picked glyph/color; resolve the task through Done/Fail/Warn/Block and let the renderer choose",
			RequiredVersion: dialectOneZero,
		})
		return true
	})
	return findings
}
