// Package review — API-061 (ZYS-974): a call site still uses the
// record-only verbs Record/RecordLabel/RecordName, removed in 1.1
// (ZYS-812). They have no record-only replacement — Decisions (2026-09-23b)
// route each call by what it actually reports: a real mutation belongs in
// evo.Effect, information (a classification/count with no state change)
// belongs in evo.Fact, and a file write belongs in evo.File/evo.Patch. The
// suggestion spells the exact rewrite when the verb literal names one.
//
// Detection is structural, by method name and call shape, matching the
// removed signatures: Record(verb string, quantity int, object string),
// RecordLabel(label string, quantity int, object string), RecordName(verb,
// object string). A call with a different arg count for that method name
// is not this detector's target and stays silent.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

// recordVerbArgCount is the exact argument count each deprecated verb
// declares, keyed by method name — the shape check that keeps this
// detector from firing on an unrelated method that happens to share a
// name.
var recordVerbArgCount = map[string]int{
	"Record":      3,
	"RecordLabel": 3,
	"RecordName":  2,
}

// detectDeprecatedRecordCall is API-061: a call to .Record/.RecordLabel/
// .RecordName whose arg count matches that verb's declared signature.
func detectDeprecatedRecordCall(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		want, isRecordVerb := recordVerbArgCount[sel.Sel.Name]
		if !isRecordVerb || len(call.Args) != want {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, deprecatedRecordCallFinding(filename, pos, sel, call.Args))
		return true
	})
	return findings
}

// recordRouting is the three-way migration every removed Record* call
// shares, appended to each suggestion so the reader sees the whole rule.
const recordRouting = "Record* has no record-only replacement (ZYS-974): route a real mutation through evo.Effect, information/classification through evo.Fact, and a file write through evo.File/evo.Patch"

// effectVerbConstants maps a Record verb literal to the EffectVerb that
// replaces it.
var effectVerbConstants = map[string]string{
	"add": "EffectAdd", "create": "EffectCreate", "delete": "EffectDelete",
	"install": "EffectInstall", "push": "EffectPush", "remove": "EffectRemove",
	"uninstall": "EffectUninstall", "update": "EffectUpdate",
}

// deprecatedRecordCallFinding builds API-061's Finding, with the exact
// rewrite when the call's verb literal determines one.
func deprecatedRecordCallFinding(filename string, pos token.Position, sel *ast.SelectorExpr, args []ast.Expr) Finding {
	verb := sel.Sel.Name
	suggestion := recordRouting
	if rewrite := recordRewrite(types.ExprString(sel.X), verb, args); rewrite != "" {
		suggestion = rewrite + "; " + recordRouting
	}
	return Finding{
		RuleID:          "API-061",
		Severity:        "warning",
		Message:         verb + " was removed in 1.1 with no record-only replacement (ZYS-974)",
		File:            filename,
		Line:            pos.Line,
		Column:          pos.Column,
		Suggestion:      suggestion,
		RequiredVersion: dialectOneOne,
	}
}

// recordRewrite spells the replacement for one removed Record* call, or ""
// when its verb is not a literal that names one.
func recordRewrite(recv, verb string, args []ast.Expr) string {
	src := make([]string, len(args))
	for i, a := range args {
		src[i] = types.ExprString(a)
	}
	if verb == "RecordLabel" {
		return "replace it with " + recv + ".Fact(" + src[0] + ", fmt.Sprintf(\"%d %s\", " + src[1] + ", " + src[2] + ")) — a classification is information, not a mutation"
	}
	lit, ok := stringLit(args[0])
	if !ok {
		return ""
	}
	object, quantity := src[len(src)-1], "1"
	if verb == "Record" {
		quantity = src[1]
	}
	if lit == "write" {
		return "move the write into " + recv + ".Define(func(ctx context.Context) error { return evo.File(ctx, evo.FileSpec{Path: " + object + ", Contents: data}) })"
	}
	constant, ok := effectVerbConstants[lit]
	if !ok {
		return "no EffectVerb is spelled " + strconv.Quote(lit) + "; pick the closest of add/create/delete/install/push/remove/uninstall/update"
	}
	return "move the mutation into " + recv + ".Define(func(ctx context.Context) error { return evo.Effect(ctx, evo.EffectSpec{Verb: evo." + constant +
		", Object: " + object + ", Quantity: " + quantity + "}, fn) })"
}
