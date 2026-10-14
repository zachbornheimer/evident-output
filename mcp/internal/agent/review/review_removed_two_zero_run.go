package review

import "go/ast"

// The 1.x package-level calls that ran, or spoke for, the whole program:
// Run (the entry that returned a Result), Fact and Confirm (run-level, now
// methods of the *evo.Run that run receives), PolicyFlag (now
// NonInteractive) and Verbose().Println (now free text on Run.Text()).

// runMethods are the package-level calls whose 2.0 spelling is the same
// method of the *evo.Run.
var runMethods = map[string]bool{"Fact": true, "Confirm": true}

// reportExecuteCall migrates evo.Run(ctx, run) and Output.Run(ctx, run),
// which returned a Result, to evo.Execute(ctx, cfg, run). The Config is the
// one the program's 1.x construction call (removed in 2.0) gave.
func (d *removedTwoZeroDetector) reportExecuteCall(call *ast.CallExpr, label string) {
	d.report(call, label+"(ctx, run) was removed in 2.0; "+d.pkg+".Execute(ctx, cfg, run) runs without signals and returns the Result",
		d.replaceRunCall(call, d.pkg+".Execute("+d.nodeSrc(call.Args[0])+", "+d.entry.hostConfig+", ", call.Args[1]))
}

// inspectResultExitCode migrates Result.Conclusion.ExitCode, a field in 1.x
// and the method Result.ExitCode in 2.0 (Conclusion has no ExitCode).
func (d *removedTwoZeroDetector) inspectResultExitCode(sel *ast.SelectorExpr) {
	conclusion, ok := sel.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ExitCode" || conclusion.Sel.Name != "Conclusion" || d.isCovered(sel) {
		return
	}
	d.report(sel, "Result.Conclusion.ExitCode was removed in 2.0; the exit code is the method Result.ExitCode()",
		"replace "+d.nodeSrc(sel)+" with "+d.nodeSrc(conclusion.X)+".ExitCode()")
}

// reportRunMethod migrates a package-level call that is a method of the
// *evo.Run in 2.0, keeping its arguments.
func (d *removedTwoZeroDetector) reportRunMethod(call *ast.CallExpr, name string) {
	run := d.runVarAt(call)
	d.report(call, d.pkg+"."+name+" was removed in 2.0; "+name+" is a method of the *"+d.pkg+".Run that run receives",
		"replace "+d.nodeSrc(call)+" with "+run+"."+name+"("+d.callArgsSrc(call.Args)+")")
}

// reportNonInteractive migrates PolicyFlag, the option that named the flag
// answering a confirmation without a person, to NonInteractive.
func (d *removedTwoZeroDetector) reportNonInteractive(call *ast.CallExpr) {
	d.report(call, d.pkg+".PolicyFlag was removed in 2.0; the flag that answers a confirmation without a person is "+d.pkg+".NonInteractive(flag)",
		"replace "+d.nodeSrc(call)+" with "+d.pkg+".NonInteractive("+d.nodeSrc(call.Args[0])+")")
}

// reportVerbosePrint migrates Verbose().Print*, 1.x text shown only at
// Verbose, to the free human text of Run.Text(). It reports whether call was
// one.
func (d *removedTwoZeroDetector) reportVerbosePrint(call *ast.CallExpr, sel *ast.SelectorExpr) bool {
	name := sel.Sel.Name
	verbose, ok := sel.X.(*ast.CallExpr)
	if !ok || printFuncs[name] == "" {
		return false
	}
	vsel, ok := verbose.Fun.(*ast.SelectorExpr)
	if !ok || vsel.Sel.Name != "Verbose" || len(verbose.Args) != 0 {
		return false
	}
	label := d.pkg + ".Verbose()"
	switch {
	case d.receiverOf(vsel.X) == outputReceiver:
		label = "Output.Verbose()"
	case !isEvoIdent(vsel.X, d.pkg):
		return false
	}
	d.reportPrint(call, label+"."+name, d.runVarAt(call), name)
	return true
}
