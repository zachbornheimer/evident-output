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
	case isEvoIdent(x, d.pkg), d.localTypes.proves(x):
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
		"Problem(\"<why>\", " + d.pkg + "." + name + "(" + d.callArgsSrc(call.Args) + ")); " +
		"if it is not, declare its type in this file so the review can prove it"
}

func (d *recSurfaceDetector) inspectRemovedRemedyMethod(call *ast.CallExpr, sel *ast.SelectorExpr, name string) bool {
	if !isRemedyMethod(name) || len(call.Args) == 0 {
		return false
	}
	kind, ok := d.remedyReceiverKind(sel.X)
	if !ok {
		return false
	}
	msg := string(kind) + "." + name + " was removed in 1.1; attach the remedy to the Problem it explains (" +
		d.pkg + "." + name + " option on Problem, Fail, or Block)"
	if kind == remedyOnUnproven {
		d.report(call, msg, d.unprovenRemedyGuidance(name, call))
		d.cover(call)
		return true
	}
	options := d.remedyOptions(name, call.Args)
	if diag := d.nearestDiagnostic(call, sel.X, kind); diag != nil {
		d.report(call, msg, d.foldSuggestion(diag, call, options))
	} else {
		fallback, owner := d.warningProblemRewrite(sel.X, kind, options)
		d.report(call, msg+"; no diagnostic in this function owns it, so "+owner, "replace "+d.nodeSrc(call)+" with "+fallback)
	}
	d.cover(call)
	return true
}

// foldSuggestion extends diag with the remedy options and drops the old call.
func (d *recSurfaceDetector) foldSuggestion(diag, next *ast.CallExpr, options []string) string {
	folded := d.extendCall(diag, options)
	return "replace " + d.nodeSrc(diag) + " with " + folded + ", then delete the statement " + d.nodeSrc(next)
}

// extendCall is call's source with options appended to its argument list.
func (d *recSurfaceDetector) extendCall(call *ast.CallExpr, options []string) string {
	src := d.nodeSrc(call)
	body := strings.TrimRight(strings.TrimSuffix(src, ")"), " \t\r\n")
	body = strings.TrimSuffix(body, ",")
	return body + ", " + strings.Join(options, ", ") + ")"
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

// nearestDiagnostic is the Fail/Block/Problem call in next's enclosing
// function that should own the remedy: for a Task, a call on the same
// receiver; for an Output, a call on any receiver. A preceding call wins over
// a following one.
func (d *recSurfaceDetector) nearestDiagnostic(next *ast.CallExpr, recv ast.Expr, kind remedyKind) *ast.CallExpr {
	body := d.enclosingFuncBody(next)
	if body == nil {
		return nil
	}
	recvSrc := d.nodeSrc(recv)
	var before, after *ast.CallExpr
	ast.Inspect(body, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || c == next {
			return true
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok || !isDiagnosticVerb(sel.Sel.Name) || isEvoIdent(sel.X, d.pkg) {
			return true
		}
		if kind == remedyOnTask && d.nodeSrc(sel.X) != recvSrc {
			return true
		}
		if c.Pos() < next.Pos() {
			before = c
		} else if after == nil {
			after = c
		}
		return true
	})
	if before != nil {
		return before
	}
	return after
}

// enclosingFuncBody is the body of the innermost function containing n.
func (d *recSurfaceDetector) enclosingFuncBody(n ast.Node) *ast.BlockStmt {
	var body *ast.BlockStmt
	ast.Inspect(d.file, func(x ast.Node) bool {
		var b *ast.BlockStmt
		switch f := x.(type) {
		case *ast.FuncDecl:
			b = f.Body
		case *ast.FuncLit:
			b = f.Body
		}
		if b != nil && b.Pos() <= n.Pos() && n.End() <= b.End() {
			body = b
		}
		return true
	})
	return body
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
