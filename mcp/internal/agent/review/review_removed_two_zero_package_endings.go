package review

import "go/ast"

// A 1.x program also spelled a Task's ending as a package-level call:
// evo.Skipped(evo.Reason(x)) and the printf form evo.Fail(format, args...).
// 2.0 has evo.Exclude(reason) and evo.Fail(summary, opts...).

// reportPackageSkipped rewrites evo.Skipped(evo.Reason(x)) to evo.Exclude(x).
func (d *removedTwoZeroDetector) reportPackageSkipped(call *ast.CallExpr) {
	d.report(call, "evo.Skipped was removed in 2.0; a policy exclusion is evo.Exclude, returned from Define",
		"replace "+d.nodeSrc(call)+" with "+d.pkg+".Exclude("+d.excludeArgs(call.Args)+")")
	d.cover(call)
}

// isPrintfFail reports whether call is the 1.x evo.Fail(format, args...): a
// second argument that is not an option, which 2.0's Fail(summary, opts...)
// does not take.
func (d *removedTwoZeroDetector) isPrintfFail(call *ast.CallExpr) bool {
	return len(call.Args) > 1 && call.Ellipsis == 0 && !d.allEvoCalls(call.Args[1:])
}

// reportPrintfFail rewrites evo.Fail(format, args...) to evo.Fail(fmt.Sprintf(format, args...)).
func (d *removedTwoZeroDetector) reportPrintfFail(call *ast.CallExpr) {
	d.report(call, "evo.Fail(format, args...) was removed in 2.0; Fail takes a summary and options, so format the summary with fmt.Sprintf",
		"replace "+d.nodeSrc(call)+" with "+d.pkg+".Fail("+d.endingArgs(call.Args, true)+")")
	d.cover(call)
}
