package review

import (
	"go/ast"
	"strconv"
	"strings"
)

// A Task ends exactly once, by returning, so every 1.x method that ended,
// annotated or narrated a Task maps onto a 2.0 return or Task method.

func (d *removedTwoZeroDetector) inspectTaskCall(call *ast.CallExpr, sel *ast.SelectorExpr) {
	name := sel.Sel.Name
	switch name {
	case "Block", "Blockf":
		d.reportReturnedEnding(call, name, "Refuse", "a precondition that stopped the work is evo.Refuse", d.endingArgs(call.Args, name == "Blockf"))
	case "Fail", "Failf":
		d.reportReturnedEnding(call, name, "Fail", "a failure is evo.Fail, returned from Define", d.endingArgs(call.Args, name == "Failf"))
	case "Skipped":
		d.reportReturnedEnding(call, name, "Exclude", "a policy exclusion is evo.Exclude", d.excludeArgs(call.Args))
	case "Kept":
		d.reportKept(call, sel)
	case "Warn":
		d.reportWarn(call, sel)
	case "Step":
		d.reportStep(call, sel)
	case "Done":
		d.reportDone(call, sel)
	case "Evidence":
		d.reportEvidence(call, sel)
	case "Next", "NextCommand":
		d.reportRemedyMethod(call, taskReceiver, name)
	default:
		d.inspectEffectCall(call, sel, name)
	}
}

// inspectOutputCall migrates what a run-level Output did. There is no
// run-level ending in 2.0, so each is guidance naming the Task that owns it.
func (d *removedTwoZeroDetector) inspectOutputCall(call *ast.CallExpr, sel *ast.SelectorExpr) {
	name := sel.Sel.Name
	switch name {
	case "Warn":
		summary := `"<why>"`
		if len(call.Args) > 0 {
			summary = d.nodeSrc(call.Args[0])
		}
		d.guide(call, outputReceiver, name, "a warning belongs to the Task it explains",
			"record the warning on that Task: <task>.Problem("+summary+")")
	case "Failf":
		d.guide(call, outputReceiver, name, "a failure ends the Task that failed",
			"return "+d.pkg+".Fail(summary) from that Task's Define callback")
	case "Next", "NextCommand":
		d.reportRemedyMethod(call, outputReceiver, name)
	}
}

// inspectFailureCall migrates Failure.Next and Failure.NextCommand.
func (d *removedTwoZeroDetector) inspectFailureCall(call *ast.CallExpr, sel *ast.SelectorExpr) {
	if name := sel.Sel.Name; name == "Next" || name == "NextCommand" {
		d.reportRemedyMethod(call, failureReceiver, name)
	}
}

// reportRemedyMethod is guidance, not an edit: a remedy is an option of the
// ending it explains, and which ending that is cannot be read from the call.
func (d *removedTwoZeroDetector) reportRemedyMethod(call *ast.CallExpr, recv removedReceiver, name string) {
	if len(call.Args) == 0 {
		return
	}
	action := d.callArgsSrc(call.Args)
	option := d.pkg + ".Next(" + d.pkg + ".Action{Label: " + action + "})"
	if name == "NextCommand" {
		option = d.nextAction(call)
	}
	d.guide(call, recv, name, "a remedy is an option of the ending it explains",
		"delete this call and pass "+option+" to the "+d.pkg+".Fail, "+d.pkg+".Refuse or Task.Problem call that explains it")
}

func (d *removedTwoZeroDetector) guide(call *ast.CallExpr, recv removedReceiver, name, instead, guidance string) {
	d.report(call, removedPhrase(recv, name)+"; "+instead, guidance)
	d.cover(call)
}

// reportReturnedEnding leaves the call uncovered: a removed evo.NextCommand
// inside its arguments is its own finding.
func (d *removedTwoZeroDetector) reportReturnedEnding(call *ast.CallExpr, name, constructor, instead, args string) {
	d.report(call, removedPhrase(taskReceiver, name)+"; "+instead,
		"replace "+d.nodeSrc(call)+" with `return "+d.pkg+"."+constructor+"("+args+")` in this Task's Define callback")
}

// endingArgs renders a removed Fail/Block call's arguments as evo.Fail and
// evo.Refuse take them: the summary, then ProblemOptions.
func (d *removedTwoZeroDetector) endingArgs(args []ast.Expr, printf bool) string {
	if len(args) == 0 {
		return `""`
	}
	summary, options := d.nodeSrc(args[0]), args[1:]
	if printf {
		if lit, ok := stringLit(args[0]); ok && strings.Contains(lit, "%w") && len(args) == 2 {
			text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(lit, "%w")), ":"))
			return strconv.Quote(text) + ", " + d.pkg + ".Detail(" + d.nodeSrc(args[1]) + ".Error())"
		}
		if len(args) > 1 && !d.allEvoCalls(options) {
			return "fmt.Sprintf(" + d.callArgsSrc(args) + ")"
		}
	}
	parts := []string{summary}
	for _, o := range options {
		if src, keep := d.problemOption(o); keep {
			parts = append(parts, src)
		}
	}
	return strings.Join(parts, ", ")
}

func (d *removedTwoZeroDetector) allEvoCalls(args []ast.Expr) bool {
	for _, a := range args {
		call, ok := a.(*ast.CallExpr)
		if !ok {
			return false
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || !isEvoIdent(sel.X, d.pkg) {
			return false
		}
	}
	return true
}

// problemOption is a 1.x option as the 2.0 Problem takes it: NextCommand
// becomes Next, Cause becomes Detail, and Severity (2.0 has none) is dropped.
func (d *removedTwoZeroDetector) problemOption(e ast.Expr) (string, bool) {
	if call, ok := d.isEvoCall(e, "NextCommand"); ok {
		return d.nextAction(call), true
	}
	if call, ok := d.isEvoCall(e, "Cause"); ok && len(call.Args) == 1 {
		return d.pkg + ".Detail(" + d.nodeSrc(call.Args[0]) + ".Error())", true
	}
	if _, ok := d.isEvoCall(e, "Severity"); ok {
		return "", false
	}
	return d.nodeSrc(e), true
}

// reportKept is guidance, not an edit: work already in place is a Verify the
// Task declares before Define, which the call's position cannot supply.
func (d *removedTwoZeroDetector) reportKept(call *ast.CallExpr, sel *ast.SelectorExpr) {
	d.guide(call, taskReceiver, "Kept", "work already in place is Task.Verify, which resolves the Task Satisfied without running Define",
		"delete this call and declare "+d.nodeSrc(sel.X)+".Verify(func(ctx context.Context) (bool, error) { return <already in place>, nil }) before Define")
}

// excludeArgs is the arguments of a Skipped call as evo.Exclude takes them:
// the reason text, unwrapped from evo.Reason.
func (d *removedTwoZeroDetector) excludeArgs(args []ast.Expr) string {
	if len(args) == 0 {
		return `"reason"`
	}
	if call, ok := d.isEvoCall(args[0], "Reason"); ok && len(call.Args) == 1 {
		return d.nodeSrc(call.Args[0])
	}
	return d.nodeSrc(args[0])
}

func (d *removedTwoZeroDetector) reportWarn(call *ast.CallExpr, sel *ast.SelectorExpr) {
	args := `""`
	if len(call.Args) > 0 {
		args = d.endingArgs(call.Args, !d.allEvoCalls(call.Args[1:]))
	}
	d.report(call, removedPhrase(taskReceiver, "Warn")+"; a warning is Task.Problem, which never ends the Task",
		"replace "+d.nodeSrc(call)+" with "+d.nodeSrc(sel.X)+".Problem("+args+")")
	d.cover(call)
}

func (d *removedTwoZeroDetector) reportStep(call *ast.CallExpr, sel *ast.SelectorExpr) {
	progress := "Progress(" + d.callArgsSrc(call.Args[:min(len(call.Args), 2)]) + ")"
	if len(call.Args) == 0 {
		progress = "Progress(0, 0)"
	}
	next := d.nodeSrc(sel.X) + "." + progress
	if len(call.Args) > 2 {
		next += ".Doing(" + d.nodeSrc(call.Args[2]) + ")"
	}
	d.report(call, removedPhrase(taskReceiver, "Step")+"; use Progress(completed, total).Doing(item)",
		"replace "+d.nodeSrc(call)+" with "+next)
	d.cover(call)
}

// reportDone maps Done onto returning from Define; the text Done carried is
// Summary.
func (d *removedTwoZeroDetector) reportDone(call *ast.CallExpr, sel *ast.SelectorExpr) {
	recv, old := d.nodeSrc(sel.X), d.nodeSrc(call)
	summary := ""
	switch len(call.Args) {
	case 0:
	case 1:
		summary = d.nodeSrc(call.Args[0])
	default:
		summary = "fmt.Sprintf(" + d.callArgsSrc(call.Args) + ")"
	}
	message := removedPhrase(taskReceiver, "Done") + "; a Task ends by returning from Define, and result text is Summary"
	switch {
	case d.insideOwnDefine(exprDottedName(sel.X), d.offset(call)) && summary == "":
		d.report(call, message, "delete "+old+"; the Define callback returning nil already ends "+recv)
	case d.insideOwnDefine(exprDottedName(sel.X), d.offset(call)):
		d.report(call, message, "replace "+old+" with "+recv+".Summary("+summary+")")
	default:
		next := recv
		if summary != "" {
			next += ".Summary(" + summary + ")"
		}
		d.report(call, message, "replace "+old+" with "+next+".Define(func(ctx context.Context) error { ... })")
	}
	d.cover(call)
}

// reportEvidence maps the capture-meaning Evidence() onto Task.Writer. A
// mutating Evidence("name", fn) is EVO-EVIDENCE-001's, not a rename.
func (d *removedTwoZeroDetector) reportEvidence(call *ast.CallExpr, sel *ast.SelectorExpr) {
	if len(call.Args) >= 2 {
		return
	}
	d.report(call, removedPhrase(taskReceiver, "Evidence")+"; the retained tail is Task.Writer(), passed as cmd.Stdout and cmd.Stderr",
		"replace "+d.nodeSrc(call)+" with "+d.nodeSrc(sel.X)+".Writer()")
	d.cover(call)
}
