// Package review — API-032 for what a 1.x program still calls: the module
// path without /v2, and Task.Block, Task.Skipped and evo.NextCommand, which
// were removed in 2.0. Each is mapped to the spelling docs/migration/2.0.md
// gives.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/modpin"
)

// dialectTwoZero is the first release that removed the 1.x module path and
// those calls (removed in 2.0).
const dialectTwoZero = "2.0.0"

// removedTwoZeroDetector reports one finding per removed construct.
type removedTwoZeroDetector struct {
	filename string
	src      string
	pkg      string
	fset     *token.FileSet
	tasks    taskBindings
	findings []Finding
}

func detectRemovedTwoZero(in fileInput) []Finding {
	pkg := evoImportName(in.file)
	if pkg == "" || !dialectAtLeast(in.desiredVersion, dialectTwoZero) {
		return nil
	}
	d := &removedTwoZeroDetector{
		filename: in.filename, src: in.src, pkg: pkg, fset: in.fset,
		tasks: newTaskBindings(in.file, pkg),
	}
	d.reportLegacyImport(in.file)
	ast.Inspect(in.file, d.inspect)
	return d.findings
}

func (d *removedTwoZeroDetector) reportLegacyImport(file *ast.File) {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != modpin.LegacyModulePath {
			continue
		}
		d.report(imp, "the module path "+path+" is the 1.x module; 2.0 is "+modpin.ModulePath,
			"replace "+imp.Path.Value+" with "+strconv.Quote(modpin.ModulePath)+
				" in every import, and run go get "+modpin.ModulePath+"@v2.0.0 to update go.mod")
	}
}

func (d *removedTwoZeroDetector) inspect(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return true
	}
	switch {
	case sel.Sel.Name == "NextCommand" && isEvoIdent(sel.X, d.pkg):
		d.report(call, "evo.NextCommand was removed in 2.0; a next step is evo.Next(evo.Action{Command: ...})",
			"replace "+d.nodeSrc(call)+" with "+d.nextAction(call))
	case sel.Sel.Name == "Block" && d.tasks.IsTask(sel.X):
		d.report(call, "Task.Block was removed in 2.0; a precondition that stopped the work is evo.Refuse",
			"replace "+d.nodeSrc(call)+" with `return "+d.pkg+".Refuse("+d.argsSrc(call.Args)+")` in this Task's Define callback")
	case sel.Sel.Name == "Skipped" && d.tasks.IsTask(sel.X):
		d.report(call, "Task.Skipped was removed in 2.0; a policy exclusion is evo.Exclude",
			"replace "+d.nodeSrc(call)+" with `return "+d.pkg+".Exclude("+d.excludeArgs(call.Args)+")` in this Task's Define callback")
	}
	return true
}

// nextAction is the 2.0 spelling of an evo.NextCommand(exe, args...) call.
func (d *removedTwoZeroDetector) nextAction(call *ast.CallExpr) string {
	command := "[]string{" + d.argsSrc(call.Args) + "}"
	if call.Ellipsis.IsValid() && len(call.Args) > 1 {
		command = "append([]string{" + d.nodeSrc(call.Args[0]) + "}, " + d.nodeSrc(call.Args[len(call.Args)-1]) + "...)"
	}
	return d.pkg + ".Next(" + d.pkg + ".Action{Command: " + command + "})"
}

// argsSrc is args as written, with each evo.NextCommand option rewritten
// to its 2.0 spelling.
func (d *removedTwoZeroDetector) argsSrc(args []ast.Expr) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = d.nodeSrc(arg)
		if call, ok := arg.(*ast.CallExpr); ok && d.isEvoCall(call, "NextCommand") {
			parts[i] = d.nextAction(call)
		}
	}
	return strings.Join(parts, ", ")
}

// excludeArgs is the arguments of a Skipped call as evo.Exclude takes them:
// the reason text, unwrapped from evo.Reason, then any options.
func (d *removedTwoZeroDetector) excludeArgs(args []ast.Expr) string {
	if len(args) == 0 {
		return `"reason"`
	}
	reason := d.nodeSrc(args[0])
	if call, ok := args[0].(*ast.CallExpr); ok && d.isEvoCall(call, "Reason") && len(call.Args) == 1 {
		reason = d.nodeSrc(call.Args[0])
	}
	return reason
}

func (d *removedTwoZeroDetector) isEvoCall(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name && isEvoIdent(sel.X, d.pkg)
}

func (d *removedTwoZeroDetector) report(n ast.Node, message, suggestion string) {
	f := d.fset.File(n.Pos())
	d.findings = append(d.findings, Finding{
		RuleID:     "API-032",
		Message:    message,
		File:       d.filename,
		Line:       lineAt(d.src, f.Offset(n.Pos())),
		Suggestion: suggestion,
	})
}

func (d *removedTwoZeroDetector) nodeSrc(n ast.Node) string {
	f := d.fset.File(n.Pos())
	return d.src[f.Offset(n.Pos()):f.Offset(n.End())]
}
