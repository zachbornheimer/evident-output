package review

import (
	"go/ast"
	"strings"
)

// 1.1 removals API-032 rewrites on dialectOneOne. Fail/Block are
// statements; Warn is Problem at SeverityWarning; Step is Progress+Doing;
// Kept is Skipped; capture-meaning Evidence* is Capture*.

func (d *recSurfaceDetector) inspectOneOneCall(call *ast.CallExpr, sel *ast.SelectorExpr, name, recv string) bool {
	if !d.effectDialect {
		return false
	}
	if d.inspectOneOneFailure(call, sel, name) {
		return true
	}
	if !isLikelyEvoReceiver(sel.X) {
		return false
	}
	if d.inspectOneOneRemovedFunc(call, sel, name) {
		return true
	}
	var (
		msg, next string
		ok        bool
	)
	switch name {
	case "Failf":
		msg = d.oneOneMethodName("Failf", recv) + " was removed in 1.1; Fail is a statement (summary, opts...)"
		next, ok = d.rewriteRemovedPrintfVerb(recv, "Fail", call)
	case "Blockf":
		msg = d.oneOneMethodName("Blockf", recv) + " was removed in 1.1; Block is a statement (summary, opts...)"
		next, ok = d.rewriteRemovedPrintfVerb(recv, "Block", call)
	case "Warn":
		msg = d.oneOneMethodName("Warn", recv) + " was removed in 1.1; a warning is Problem at SeverityWarning"
		next, ok = d.rewriteRemovedWarn(recv, call)
	case "Step":
		msg = d.oneOneMethodName("Step", recv) + " was removed in 1.1; use Progress(completed, total).Doing(item)"
		next, ok = d.rewriteRemovedStep(recv, call)
	case "Kept":
		msg = d.oneOneMethodName("Kept", recv) + " was removed in 1.1; policy exclusion is Skipped(Reason(...))"
		next, ok = d.rewriteRemovedKept(recv, call)
	case "Evidence":
		if d.isMutatingEvidence(call) {
			return false
		}
		if recv != "" && !isEvoSurfaceRecv(recv) && (d.doneScope == nil || !d.doneScope.tasks.IsTask(sel.X)) {
			return false
		}
		msg = "Evidence was removed in 1.1; the retained sink is Capture"
		next, ok = d.rewriteRemovedEvidence(recv, call)
	default:
		return false
	}
	if !ok {
		return false
	}
	old := d.nodeSrc(call)
	d.report(call, msg, "replace "+old+" with "+next)
	d.cover(call)
	return true
}

func (d *recSurfaceDetector) oneOneMethodName(name, recv string) string {
	switch recv {
	case "", d.pkg, "evo":
		return name
	case "out", "output", "o":
		return "Output." + name
	default:
		return "TaskHandle." + name
	}
}

func (d *recSurfaceDetector) inspectOneOneRemovedFunc(call *ast.CallExpr, sel *ast.SelectorExpr, name string) bool {
	if !isEvoIdent(sel.X, d.pkg) {
		return false
	}
	var (
		msg, next string
		ok        bool
	)
	switch name {
	case "EncodeJSON":
		msg = "EncodeJSON was removed in 1.1; write the run document with WriteJSON"
		next, ok = d.renameEvoCall(call, "WriteJSON")
	case "EncodeJSONL":
		msg = "EncodeJSONL was removed in 1.1; stream events with FormatJSONL"
		next = d.pkg + ".Init(" + d.pkg + ".Config{Format: " + d.pkg + ".FormatJSONL})"
		ok = true
	case "EncodeEventJSON":
		msg = "EncodeEventJSON was removed in 1.1; stream events with FormatJSONL"
		next = d.pkg + ".Init(" + d.pkg + ".Config{Format: " + d.pkg + ".FormatJSONL})"
		ok = true
	case "ForSkip", "OnTask":
		msg = name + " was removed in 1.1; name the skip with Reason and resolve with Skipped"
		next = d.pkg + `.Reason("skip")`
		ok = true
	case "ReasonOption":
		msg = "ReasonOption was removed in 1.1; construct a skip with Reason"
		next = d.pkg + `.Reason("skip")`
		ok = true
	default:
		return false
	}
	if !ok {
		return false
	}
	old := d.nodeSrc(call)
	d.report(call, msg, "replace "+old+" with "+next)
	d.cover(call)
	return true
}

func (d *recSurfaceDetector) renameEvoCall(call *ast.CallExpr, newName string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	return d.nodeSrc(sel.X) + "." + newName + "(" + d.callArgsSrc(call.Args) + ")", true
}

func (d *recSurfaceDetector) rewriteRemovedPrintfVerb(recv, verb string, call *ast.CallExpr) (string, bool) {
	if recv == "" {
		recv = d.nodeSrc(call.Fun.(*ast.SelectorExpr).X)
	}
	if recv == "" {
		return "", false
	}
	return recv + "." + verb + "(" + d.failSummaryArgs(call.Args) + ")", true
}

func (d *recSurfaceDetector) failSummaryArgs(args []ast.Expr) string {
	if len(args) == 0 {
		return `""`
	}
	if lit, ok := stringLit(args[0]); ok && strings.Contains(lit, "%w") && len(args) >= 2 {
		summary := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(lit, "%w"), ":"))
		errExpr := d.nodeSrc(args[len(args)-1])
		var out strings.Builder
		out.WriteString(`"` + summary + `", ` + d.pkg + `.Detail(` + errExpr + `.Error())`)
		for _, a := range args[1 : len(args)-1] {
			out.WriteString(", " + d.nodeSrc(a))
		}
		return out.String()
	}
	if len(args) == 1 {
		return d.nodeSrc(args[0])
	}
	if _, ok := stringLit(args[0]); ok && !d.argsAreProblemOptions(args[1:]) {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = d.nodeSrc(a)
		}
		return "fmt.Sprintf(" + strings.Join(parts, ", ") + ")"
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = d.nodeSrc(a)
	}
	return strings.Join(parts, ", ")
}

func (d *recSurfaceDetector) rewriteRemovedWarn(recv string, call *ast.CallExpr) (string, bool) {
	if recv == "" {
		recv = d.nodeSrc(call.Fun.(*ast.SelectorExpr).X)
	}
	switch recv {
	case "", "evo", d.pkg, "out", "output", "o":
		recv = "task"
	}
	sev := d.pkg + ".Severity(" + d.pkg + ".SeverityWarning)"
	if len(call.Args) == 0 {
		return recv + ".Problem(\"\", " + sev + ")", true
	}
	summary, opts := d.splitSummaryAndOptions(call.Args)
	var next strings.Builder
	next.WriteString(recv + ".Problem(" + summary + ", " + sev)
	for _, o := range opts {
		next.WriteString(", " + o)
	}
	return next.String() + ")", true
}

func (d *recSurfaceDetector) rewriteRemovedStep(recv string, call *ast.CallExpr) (string, bool) {
	if recv == "" {
		recv = d.nodeSrc(call.Fun.(*ast.SelectorExpr).X)
	}
	if recv == "" {
		return "", false
	}
	switch len(call.Args) {
	case 0:
		return recv + ".Progress(0, 0)", true
	case 1:
		return recv + ".Progress(" + d.nodeSrc(call.Args[0]) + ", 0)", true
	case 2:
		return recv + ".Progress(" + d.nodeSrc(call.Args[0]) + ", " + d.nodeSrc(call.Args[1]) + ")", true
	default:
		return recv + ".Progress(" + d.nodeSrc(call.Args[0]) + ", " + d.nodeSrc(call.Args[1]) + ").Doing(" + d.nodeSrc(call.Args[2]) + ")", true
	}
}

func (d *recSurfaceDetector) rewriteRemovedKept(recv string, call *ast.CallExpr) (string, bool) {
	if recv == "" {
		recv = d.nodeSrc(call.Fun.(*ast.SelectorExpr).X)
	}
	if recv == "" {
		return "", false
	}
	args := ""
	if len(call.Args) > 0 {
		parts := make([]string, len(call.Args))
		for i, a := range call.Args {
			parts[i] = d.nodeSrc(a)
		}
		args = strings.Join(parts, ", ")
	}
	return recv + ".Skipped(" + args + ")", true
}

func (d *recSurfaceDetector) rewriteRemovedEvidence(recv string, call *ast.CallExpr) (string, bool) {
	if recv == "" {
		recv = d.nodeSrc(call.Fun.(*ast.SelectorExpr).X)
	}
	if recv == "" {
		return "", false
	}
	args := ""
	if len(call.Args) > 0 {
		parts := make([]string, len(call.Args))
		for i, a := range call.Args {
			parts[i] = d.nodeSrc(a)
		}
		args = strings.Join(parts, ", ")
	}
	return recv + ".Capture(" + args + ")", true
}

func (d *recSurfaceDetector) isMutatingEvidence(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	if _, ok := stringLit(call.Args[0]); !ok {
		return false
	}
	_, isFn := call.Args[1].(*ast.FuncLit)
	if ident, ok := call.Args[1].(*ast.Ident); ok && ident.Name == "nil" {
		isFn = true
	}
	return isFn
}

func (d *recSurfaceDetector) splitSummaryAndOptions(args []ast.Expr) (string, []string) {
	if len(args) == 0 {
		return `""`, nil
	}
	if d.argsAreProblemOptions(args[1:]) {
		opts := make([]string, len(args)-1)
		for i, a := range args[1:] {
			opts[i] = d.nodeSrc(a)
		}
		return d.nodeSrc(args[0]), opts
	}
	if len(args) == 1 {
		return d.nodeSrc(args[0]), nil
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = d.nodeSrc(a)
	}
	return "fmt.Sprintf(" + strings.Join(parts, ", ") + ")", nil
}

func (d *recSurfaceDetector) argsAreProblemOptions(args []ast.Expr) bool {
	if len(args) == 0 {
		return true
	}
	for _, a := range args {
		call, ok := a.(*ast.CallExpr)
		if !ok {
			return false
		}
		_, _, ok = d.evoCall(call)
		if !ok {
			return false
		}
	}
	return true
}
