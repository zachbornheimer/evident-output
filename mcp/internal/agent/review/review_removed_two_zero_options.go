package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// The 1.x constructors of an option or an action that 2.0 replaced by a
// different value, per the "Problems, remedies and confirmations" table of
// docs/migration/2.0.md: Location (evo.On), Count and ConfirmDetail
// (evo.Detail) and Command and Label (an evo.Action literal). A constructor whose
// arguments do not map one-to-one gets no rewrite, so the type error review
// reports on the 2.0 package still names the guide's replacement.

// removedValueCalls are the package-level constructors this arm rewrites.
var removedValueCalls = map[string]bool{"Location": true, "Count": true, "ConfirmDetail": true, "Command": true, "Label": true}

// inspectRemovedValueCall migrates one such call wherever it stands.
func (d *removedTwoZeroDetector) inspectRemovedValueCall(call *ast.CallExpr, name string) {
	spelling, ok := d.valueCallSpelling(call, name)
	if !ok {
		return
	}
	d.report(call, "evo."+name+" was removed in 2.0; "+removedValueInstead[name], "replace "+d.nodeSrc(call)+" with "+spelling)
	d.cover(call)
}

var removedValueInstead = map[string]string{
	"Location":      `a subject is evo.On("path:line:col")`,
	"Count":         "a quantity is text in evo.Detail(text)",
	"ConfirmDetail": "a context line is evo.Detail(text)",
	"Command":       "an action is evo.Action{Command: []string{...}}",
	"Label":         "a text-only action is evo.Action{Label: text}",
}

// valueCallSpelling is call as 2.0 spells it. ok is false when the arguments
// have no one-to-one spelling (a variadic slice passed whole).
func (d *removedTwoZeroDetector) valueCallSpelling(call *ast.CallExpr, name string) (string, bool) {
	if call.Ellipsis.IsValid() && name != "Command" {
		return "", false
	}
	switch {
	case name == "Location" && len(call.Args) == 3:
		return d.pkg + ".On(" + d.locationText(call.Args) + ")", true
	case name == "Count" && (len(call.Args) == 1 || len(call.Args) == 2):
		return d.pkg + ".Detail(" + d.countText(call.Args) + ")", true
	case name == "ConfirmDetail" && len(call.Args) > 0:
		details := make([]string, len(call.Args))
		for i, line := range call.Args {
			details[i] = d.pkg + ".Detail(" + d.nodeSrc(line) + ")"
		}
		return strings.Join(details, ", "), true
	case name == "Command" && len(call.Args) > 0:
		return d.actionLiteral(call), true
	case name == "Label" && len(call.Args) == 1:
		return d.pkg + ".Action{Label: " + d.nodeSrc(call.Args[0]) + "}", true
	}
	return "", false
}

// actionLiteral is an evo.Command(exe, args...) call as the 2.0 Action.
func (d *removedTwoZeroDetector) actionLiteral(call *ast.CallExpr) string {
	command := "[]string{" + d.callArgsSrc(call.Args) + "}"
	if call.Ellipsis.IsValid() && len(call.Args) > 1 {
		command = "append([]string{" + d.nodeSrc(call.Args[0]) + "}, " + d.nodeSrc(call.Args[len(call.Args)-1]) + "...)"
	}
	return d.pkg + ".Action{Command: " + command + "}"
}

// locationText is the "path:line:col" subject: one string literal when every
// argument is a literal, else a Sprintf over them.
func (d *removedTwoZeroDetector) locationText(args []ast.Expr) string {
	return d.joinedText(args, "%s:%d:%d", ":")
}

// countText is the "<n> <unit>" text of a Count.
func (d *removedTwoZeroDetector) countText(args []ast.Expr) string {
	if len(args) == 1 {
		return d.joinedText(args, "%d", " ")
	}
	return d.joinedText(args, "%d %s", " ")
}

// joinedText is the string the args make when joined by sep: a single
// literal when every argument is one, else fmt.Sprintf(format, args...).
func (d *removedTwoZeroDetector) joinedText(args []ast.Expr, format, sep string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		lit, ok := a.(*ast.BasicLit)
		if !ok || (lit.Kind != token.INT && lit.Kind != token.STRING) {
			d.need("fmt")
			return "fmt.Sprintf(" + strconv.Quote(format) + ", " + d.callArgsSrc(args) + ")"
		}
		parts[i] = lit.Value
		if lit.Kind == token.STRING {
			if unquoted, err := strconv.Unquote(lit.Value); err == nil {
				parts[i] = unquoted
			}
		}
	}
	return strconv.Quote(strings.Join(parts, sep))
}

// confirmOptionSpelling is a 1.x Confirm option as 2.0 takes it, when it has
// a one-to-one spelling; AssumeYes has none (see confirmCallSpelling).
func (d *removedTwoZeroDetector) confirmOptionSpelling(e ast.Expr) (string, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	if flag, ok := d.isEvoCall(call, "PolicyFlag"); ok && len(flag.Args) == 1 {
		return d.pkg + ".NonInteractive(" + d.nodeSrc(flag.Args[0]) + ")", true
	}
	if _, ok := d.isEvoCall(call, "ConfirmDetail"); ok {
		return d.valueCallSpelling(call, "ConfirmDetail")
	}
	return "", false
}

// confirmCallSpelling is a Confirm call as a method of run. 2.0 never answers
// yes by itself, so AssumeYes(v) becomes the condition the program tests
// before it asks: (v || run.Confirm(...)).
func (d *removedTwoZeroDetector) confirmCallSpelling(call *ast.CallExpr, run string) string {
	var assumed []string
	args := make([]string, 0, len(call.Args))
	for i, arg := range call.Args {
		if yes, ok := d.isEvoCall(arg, "AssumeYes"); ok && i > 0 && len(yes.Args) == 1 {
			assumed = append(assumed, d.nodeSrc(yes.Args[0]))
			continue
		}
		if spelling, ok := d.confirmOptionSpelling(arg); ok && i > 0 {
			args = append(args, spelling)
			continue
		}
		args = append(args, d.nodeSrc(arg))
	}
	confirm := run + ".Confirm(" + strings.Join(args, ", ") + ")"
	if len(assumed) == 0 {
		return confirm
	}
	return "(" + strings.Join(assumed, " || ") + " || " + confirm + ")"
}
