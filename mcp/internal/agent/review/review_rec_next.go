package review

import (
	"go/ast"
	"regexp"
	"strings"
)

// ZYS-1182: a remedy belongs to the Problem it explains. TaskHandle.Next,
// TaskHandle.NextCommand, Output.Next, and Output.NextCommand were removed;
// Next and NextCommand exist only as ProblemOptions.
//
// The rewrite prefers folding the option into a Fail/Block/Problem call the
// same function already makes. Only when there is none does it introduce a
// warning Problem, whose summary is a placeholder the author must replace;
// detectRemedyPlaceholder keeps the finding open until they do.

// remedyWarningSummary is the placeholder Problem summary the fallback rewrite
// introduces.
const remedyWarningSummary = "TODO state why this remedy is suggested"

// remedyOutputTaskName is the Task an Output-level remedy lands on when the
// function declares no diagnostic to own it.
const remedyOutputTaskName = "next steps"

// remedyKind is which removed receiver a Next call was made on.
type remedyKind string

const (
	remedyOnTask   remedyKind = "TaskHandle"
	remedyOnOutput remedyKind = "Output"
	// remedyOnUnproven is a receiver the file neither proves is an evo
	// TaskHandle or Output nor proves is something else.
	remedyOnUnproven remedyKind = "TaskHandle or Output"
)

func isRemedyMethod(name string) bool { return name == "Next" || name == "NextCommand" }

func isDiagnosticVerb(name string) bool {
	return name == "Fail" || name == "Block" || name == "Problem"
}

// remedyReceiverKind classifies x. The package-level evo.Next/evo.NextCommand
// and receivers proven to be non-evo local types are not remedy calls
// (ok=false). Every other receiver is reported: a Task or Output, or
// remedyOnUnproven when the file cannot say which (a field declared in another
// file, a helper's return value, a fluent chain).
func (d *recSurfaceDetector) remedyReceiverKind(x ast.Expr) (remedyKind, bool) {
	switch {
	case isEvoIdent(x, d.pkg), d.declaredTypes.proves(x):
		return "", false
	case d.doneScope != nil && d.doneScope.tasks.IsTask(x):
		return remedyOnTask, true
	case d.outputs.has(x), isOutputConstructor(x, d.pkg):
		return remedyOnOutput, true
	}
	return remedyOnUnproven, true
}

// unprovenRemedyGuidance is the suggestion when no single compile-valid edit is
// known; it is deliberately not a "replace ... with ..." line.
func (d *recSurfaceDetector) unprovenRemedyGuidance(name string, call *ast.CallExpr) string {
	return "if the receiver is an evo Task or Output, move the remedy onto the Problem it explains: " +
		"Problem(\"<why>\", " + d.pkg + "." + name + "(" + d.callArgsSrc(call.Args) + spreadMark(call) + ")); " +
		"if it is not, the review could not see its type (another file, a call result, a chain): " +
		"give the variable an explicit non-evo type in this file so the review can prove it"
}

func (d *recSurfaceDetector) inspectRemovedRemedyMethod(call *ast.CallExpr, sel *ast.SelectorExpr, name string) bool {
	if !isRemedyMethod(name) {
		return false
	}
	kind, ok := d.remedyReceiverKind(sel.X)
	if !ok {
		return false
	}
	msg := string(kind) + "." + name + " was removed in 1.1; attach the remedy to the Problem it explains (" +
		d.pkg + "." + name + " option on Problem, Fail, or Block)"
	if len(call.Args) == 0 {
		// A bare Next() is the iterator idiom on any unproven receiver
		// (sql.Rows, a cursor); only a proven evo receiver is a removed call.
		if kind == remedyOnUnproven {
			return false
		}
		d.report(call, msg+"; with no action there is no remedy to attach, so delete the call", "delete "+d.nodeSrc(call)+"; add "+d.pkg+"."+name+"(...) to the Problem it explains only if a remedy is intended")
		d.cover(call)
		return true
	}
	if kind == remedyOnUnproven {
		d.report(call, msg, d.unprovenRemedyGuidance(name, call))
		d.cover(call)
		return true
	}
	stmt, standalone := d.standaloneStatement(call)
	if !standalone || call.Ellipsis.IsValid() {
		d.report(call, msg, d.unprovenRemedyGuidance(name, call))
		d.cover(call)
		return true
	}
	d.reportRemedyRewrite(call, sel, kind, stmt, msg)
	d.cover(call)
	return true
}

// reportRemedyRewrite picks the first rewrite that keeps the remedy on the
// path it belongs to: fold into a sibling diagnostic on the same receiver, a
// Fail at the error return of the Task's Define callback (which still returns
// the error), or a placeholder warning Problem.
func (d *recSurfaceDetector) reportRemedyRewrite(call *ast.CallExpr, sel *ast.SelectorExpr, kind remedyKind, stmt remedyStatement, msg string) {
	options := d.remedyOptions(sel.Sel.Name, call.Args)
	if diag := d.adjacentDiagnostic(stmt, d.nodeSrc(sel.X)); diag != nil {
		d.reportEdit(call, msg, d.foldEdit(diag, call, options))
		return
	}
	if kind == remedyOnTask {
		if edit, ok := d.defineFailRewrite(call, sel.X, stmt, options); ok {
			d.reportEdit(call, msg+"; this Define callback returns an error right after it, so Fail records the remedy as a diagnostic and the callback still returns the error (Wait() reports it unchanged)", edit)
			return
		}
	}
	fallback, owner := d.warningProblemRewrite(sel.X, kind, options)
	span := d.nodeSpan(call)
	d.reportEdit(call, msg+"; no adjacent diagnostic owns it, so "+owner, remedyEdit{start: span.start, end: span.end, right: fallback})
}

// reportEdit reports edit as one single-line replace anchored at the line the
// replaced text starts on. An edit that cannot be written on one line without
// altering the code (a comment or a multi-line raw string inside it) is
// declined: the finding stays open with guidance for a by-hand edit.
func (d *recSurfaceDetector) reportEdit(call *ast.CallExpr, msg string, edit remedyEdit) {
	if suggestion, ok := replaceSuggestion(d.src[edit.start:edit.end], edit.right); ok {
		d.reportAt(edit.start, msg, suggestion)
		return
	}
	name := call.Fun.(*ast.SelectorExpr).Sel.Name
	d.report(call, msg+"; the edit would cross a comment or a multi-line raw string, so no one-line rewrite is offered",
		"edit by hand: add "+d.pkg+"."+name+"("+d.callArgsSrc(call.Args)+") to the Fail, Block, or Problem call it explains, then delete this call")
}

// spreadMark is "..." when call's last argument is spread.
func spreadMark(call *ast.CallExpr) string {
	if call.Ellipsis.IsValid() {
		return "..."
	}
	return ""
}

// warningProblemRewrite builds the fallback and names the owner it chose.
func (d *recSurfaceDetector) warningProblemRewrite(recv ast.Expr, kind remedyKind, options []string) (rewrite, owner string) {
	target := d.nodeSrc(recv)
	owner = "it becomes a warning Problem on this Task; replace the placeholder summary"
	if kind == remedyOnOutput {
		target += ".Task(\"" + remedyOutputTaskName + "\")"
		owner = "it lands on a \"" + remedyOutputTaskName + "\" Task (an Output has no natural owner); name the Task it explains and replace the placeholder summary"
	}
	all := append([]string{d.pkg + ".Severity(" + d.pkg + ".SeverityWarning)"}, options...)
	return target + ".Problem(\"" + remedyWarningSummary + "\", " + strings.Join(all, ", ") + ")", owner
}

// remedyOptions renders the removed call's arguments as ProblemOptions.
func (d *recSurfaceDetector) remedyOptions(name string, args []ast.Expr) []string {
	if name == "NextCommand" {
		return []string{d.pkg + ".NextCommand(" + d.callArgsSrc(args) + ")"}
	}
	options := make([]string, len(args))
	for i, a := range args {
		action := d.nodeSrc(a)
		if _, ok := stringLit(a); ok {
			action = d.pkg + ".Label(" + action + ")"
		}
		options[i] = d.pkg + ".Next(" + action + ")"
	}
	return options
}

// remedyPlaceholderLiteral matches the fallback rewrite's placeholder summary
// left in source.
var remedyPlaceholderLiteral = regexp.MustCompile(`"` + regexp.QuoteMeta(remedyWarningSummary) + `"`)

// detectRemedyPlaceholder keeps the review open while a fallback rewrite's
// placeholder summary is still in the source, so it cannot ship unedited.
func detectRemedyPlaceholder(filename, src string) []Finding {
	var findings []Finding
	for _, m := range remedyPlaceholderLiteral.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Message:    "placeholder Problem summary from the removed-Next rewrite; state why this remedy is suggested",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace the placeholder with the reason the remedy is suggested, or fold the option into the Fail/Block call it explains",
		})
	}
	return findings
}
