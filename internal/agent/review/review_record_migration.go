// Package review — API-060 (ZYS-974): a call site still uses the
// record-only mutation verbs Record/RecordLabel/RecordName. Those three
// have no record-only replacement — Decisions (2026-09-23b) route each
// call by what it actually reports: a real mutation belongs in evo.Effect,
// information (a classification/count with no state change) belongs in
// evo.Fact, and a file write belongs in evo.File/evo.Patch. This detector
// steers a caller at the exact call site rather than leaving the
// migration to doc prose only.
//
// Detection is structural, by method name and call shape, matching the
// same signatures TaskHandle.Record/RecordLabel/RecordName declare:
// Record(verb string, quantity int, object string), RecordLabel(label
// string, quantity int, object string), RecordName(verb, object string).
// A call with a different arg count for that method name is not this
// detector's target (it cannot be the deprecated verb) and stays silent.
package review

import (
	"go/ast"
	"go/token"
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

// detectDeprecatedRecordCall is API-060: a call to .Record/.RecordLabel/
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
		findings = append(findings, deprecatedRecordCallFinding(filename, pos, sel.Sel.Name))
		return true
	})
	return findings
}

// deprecatedRecordCallFinding builds API-060's Finding. verb names the
// deprecated method (Record, RecordLabel, or RecordName) so the
// suggestion can point at that call's most likely replacement.
func deprecatedRecordCallFinding(filename string, pos token.Position, verb string) Finding {
	suggestion := "Record has no record-only replacement (ZYS-974): route a real mutation through evo.Effect, information/classification through evo.Fact, and a file write through evo.File/evo.Patch"
	if verb == "RecordLabel" {
		suggestion = "RecordLabel classifies rather than mutates; migrate to evo.Fact (information) — it has no record-only replacement (ZYS-974)"
	}
	return Finding{
		RuleID:          "API-060",
		Severity:        "warning",
		Message:         verb + " is deprecated with no record-only replacement (ZYS-974)",
		File:            filename,
		Line:            pos.Line,
		Column:          pos.Column,
		Suggestion:      suggestion,
		RequiredVersion: dialectOneOne,
	}
}
