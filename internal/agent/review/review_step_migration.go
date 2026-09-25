// Package review — API-090 (E-119): a call site still uses TaskHandle.Step,
// removed in 1.1 with no alias. A count's current item is orthogonal to the
// count itself, so the canonical spelling is Progress(completed,
// total).Doing(item); the suggestion spells that chain from the call's own
// arguments.
//
// Detection is structural: the method is Step with Step's three arguments,
// and the receiver traces back to an evo Task value (see taskBindings). A
// Step method on anything else stays silent.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
)

// removedStepArgCount is the argument count of the removed
// Step(completed, total, name) signature.
const removedStepArgCount = 3

// detectRemovedStepCall is API-090.
func detectRemovedStepCall(filename string, file *ast.File, fset *token.FileSet) []Finding {
	evoPkg := evoImportName(file)
	if evoPkg == "" {
		return nil
	}
	tasks := newTaskBindings(file, evoPkg)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Step" || len(call.Args) != removedStepArgCount || !tasks.IsTask(sel.X) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-090",
			Message:    "TaskHandle.Step was removed in 1.1; the current item of a count is Progress(completed, total).Doing(item)",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace it with " + stepRewrite(sel.X, call.Args),
		})
		return true
	})
	return findings
}

// stepRewrite spells recv.Progress(completed, total).Doing(name) from one
// removed Step call.
func stepRewrite(recv ast.Expr, args []ast.Expr) string {
	return types.ExprString(recv) + ".Progress(" + types.ExprString(args[0]) + ", " +
		types.ExprString(args[1]) + ").Doing(" + types.ExprString(args[2]) + ")"
}
