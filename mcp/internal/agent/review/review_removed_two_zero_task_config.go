package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// Two fluent Task settings of 1.x with no 2.0 method: Key (the Task name is
// its identity) and Bytes (progress is counted, whatever the unit).

// reportKey drops a Task's Key. As a statement of its own it is deleted; in
// a chain the call leaves its receiver.
func (d *removedTwoZeroDetector) reportKey(call *ast.CallExpr, sel *ast.SelectorExpr) {
	message := removedPhrase(taskReceiver, "Key") + "; the Task name is its identity"
	if stmt := d.stmtOf(call); stmt != nil {
		d.report(call, message, "delete "+d.nodeSrc(stmt)+"; the Task name is its identity")
	} else {
		d.report(call, message, "replace "+d.nodeSrc(call)+" with "+d.recvSrc(sel.X))
	}
	d.cover(call)
}

// reportBytes migrates Bytes(completed, total int64) to Progress(completed,
// total int); an operand that is not a literal is converted.
func (d *removedTwoZeroDetector) reportBytes(call *ast.CallExpr, sel *ast.SelectorExpr) {
	args := make([]string, len(call.Args))
	for i, a := range call.Args {
		args[i] = d.nodeSrc(a)
		if lit, ok := a.(*ast.BasicLit); !ok || lit.Kind != token.INT {
			args[i] = "int(" + args[i] + ")"
		}
	}
	d.report(call, removedPhrase(taskReceiver, "Bytes")+"; progress is Task.Progress(completed, total), whatever the unit",
		"replace "+d.nodeSrc(call)+" with "+d.recvSrc(sel.X)+".Progress("+strings.Join(args, ", ")+")")
	d.cover(call)
}
