package review

import (
	"go/ast"
	"strings"
)

// oneOneTypeNext maps a 1.1-removed evo type or value onto the live
// identifier that replaces it. An empty next means drop the evo export
// (EventSchemaVersion lives on the wire document).
var oneOneTypeNext = map[string]string{
	"EvidenceOption":         "CaptureOption",
	"EvidenceStream":         "CaptureStream",
	"EvidenceStreamCombined": "CaptureStreamCombined",
	"EvidenceStreamStdout":   "CaptureStreamStdout",
	"EvidenceStreamStderr":   "CaptureStreamStderr",
	"MaxEvidenceBytes":       "MaxCaptureBytes",
	"Option":                 "Config",
	"ReasonOption":           "Reason",
	"ErrReasonSkipOnly":      "Reason",
	"ErrReasonWrongTask":     "Reason",
	"ConclusionJSON":         "WriteJSON",
	"EventJSON":              "FormatJSONL",
	"EventSchemaVersion":     "",
	"JSONAction":             "WriteJSON",
	"JSONChanges":            "WriteJSON",
	"JSONCollection":         "WriteJSON",
	"JSONCommand":            "WriteJSON",
	"JSONDocument":           "WriteJSON",
	"JSONEffectRecord":       "WriteJSON",
	"JSONMessage":            "WriteJSON",
	"JSONOutputMeta":         "WriteJSON",
	"JSONPlan":               "WriteJSON",
	"JSONProblem":            "WriteJSON",
	"JSONProgress":           "WriteJSON",
	"JSONSchemaVersion":      "WriteJSON",
	"JSONTask":               "WriteJSON",
}

func (d *recSurfaceDetector) inspectOneOneType(n ast.Node) bool {
	if !d.effectDialect {
		return false
	}
	sel, ok := n.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, d.pkg) {
		return false
	}
	next, known := oneOneTypeNext[sel.Sel.Name]
	if !known || d.isCovered(sel) {
		return false
	}
	old := d.nodeSrc(sel)
	msg := sel.Sel.Name + " was removed in 1.1; " + oneOneTypeNote(sel.Sel.Name, next)
	sug := ""
	if next != "" {
		sug = "replace " + old + " with " + d.pkg + "." + next
	}
	d.report(sel, msg, sug)
	d.cover(sel)
	return true
}

func oneOneTypeNote(name, next string) string {
	switch {
	case name == "EventSchemaVersion":
		return "the schema version lives on the wire document, not an evo export"
	case name == "Option":
		return "construct with Config fields"
	case stringsHasPrefixJSON(name) || name == "ConclusionJSON" || name == "EventJSON":
		return "encode with WriteJSON or Config.Format = " + next
	case name == "ErrReasonSkipOnly" || name == "ErrReasonWrongTask" || name == "ReasonOption":
		return "name a skip with Reason and resolve with Skipped"
	default:
		return "the capture sink uses " + next
	}
}

func stringsHasPrefixJSON(name string) bool {
	return len(name) >= 4 && name[:4] == "JSON"
}

// failureBindings is which names in one file are typed evo.Failure.
type failureBindings struct {
	names map[string]bool
}

func newFailureBindings(file *ast.File, evoPkg string) failureBindings {
	b := failureBindings{names: map[string]bool{}}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			if isFailureType(n.Type, evoPkg) {
				for _, name := range n.Names {
					b.names[name.Name] = true
				}
			}
		case *ast.ValueSpec:
			if n.Type != nil && isFailureType(n.Type, evoPkg) {
				for _, name := range n.Names {
					b.names[name.Name] = true
				}
			}
		}
		return true
	})
	return b
}

func (b failureBindings) has(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && b.names[id.Name]
}

func isFailureType(t ast.Expr, evoPkg string) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch x := t.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "Failure" && isEvoIdent(x.X, evoPkg)
	case *ast.Ident:
		return x.Name == "Failure"
	default:
		return false
	}
}

func (d *recSurfaceDetector) inspectOneOneFailure(call *ast.CallExpr, sel *ast.SelectorExpr, name string) bool {
	if !d.effectDialect || (name != "Next" && name != "NextCommand") {
		return false
	}
	if !d.failures.has(sel.X) {
		return false
	}
	args := d.callArgsSrc(call.Args)
	if name == "Next" {
		args = d.failureNextArgs(call.Args)
	}
	old := d.nodeSrc(call)
	next := "task." + name + "(" + args + ")"
	d.report(call, "Failure."+name+" was removed in 1.1; call TaskHandle."+name+" after Fail or Block",
		"replace "+old+" with "+next)
	d.cover(call)
	return true
}

func (d *recSurfaceDetector) failureNextArgs(args []ast.Expr) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		if _, ok := stringLit(a); ok {
			parts[i] = d.pkg + ".Label(" + d.nodeSrc(a) + ")"
			continue
		}
		parts[i] = d.nodeSrc(a)
	}
	return joinComma(parts)
}

func (d *recSurfaceDetector) callArgsSrc(args []ast.Expr) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = d.nodeSrc(a)
	}
	return joinComma(parts)
}

func joinComma(parts []string) string {
	var out strings.Builder
	out.WriteString(parts[0])
	for _, p := range parts[1:] {
		out.WriteString(", " + p)
	}
	return out.String()
}
