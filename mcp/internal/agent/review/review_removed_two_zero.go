// Package review — API-032 for what a 1.x program still calls. This is the
// migration layer: dirty 1.x input is the point, and every finding lands
// directly on the 2.0 surface docs/migration/2.0.md names, never on a 1.x
// intermediate. The arms are split by what they migrate: the module path and
// package calls (here), Task and Output methods (review_removed_two_zero_tasks.go),
// mutation and record verbs (…_effects.go), Option funcs and Config.Options
// (…_config.go), and every other removed export by its retired-table
// replacement (…_exports.go).
package review

import (
	"go/ast"
	"go/token"
	"regexp"
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
	file     *ast.File
	tasks    taskBindings
	outputs  outputBindings
	failures failureBindings
	// defineBodies are the Define callbacks, so Done inside its own Task's
	// callback is rewritten differently from Done outside it.
	defineBodies []defineBody
	covered      []srcSpan
	reported     []srcSpan
	entry        entryScan
	findings     []Finding
}

func detectRemovedTwoZero(in fileInput) []Finding {
	pkg := evoImportName(in.file)
	if pkg == "" || !dialectAtLeast(in.desiredVersion, dialectTwoZero) {
		return nil
	}
	d := &removedTwoZeroDetector{
		filename: in.filename, src: in.src, pkg: pkg, fset: in.fset, file: in.file,
		tasks:        newTaskBindings(in.file, pkg),
		outputs:      newOutputBindings(in.file, pkg),
		failures:     newFailureBindings(in.file, pkg),
		defineBodies: collectDefineBodies(in.file, in.fset),
	}
	d.scanEntryPoints()
	d.scanExecResults()
	d.reportLegacyImport(in.file)
	ast.Inspect(in.file, d.inspect)
	ast.Inspect(in.file, d.inspectRemovedExport)
	d.reportMissingImports()
	d.reportCompileErrors()
	return d.findings
}

func (d *removedTwoZeroDetector) reportLegacyImport(file *ast.File) {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != modpin.LegacyModulePath {
			continue
		}
		d.report(imp, "the module path "+path+" is the 1.x module; 2.0 is "+modpin.ModulePath+
			"; change every import, then run go get "+modpin.ModulePath+"@v2.0.0 to update go.mod",
			"replace "+imp.Path.Value+" with "+strconv.Quote(modpin.ModulePath))
	}
}

func (d *removedTwoZeroDetector) inspect(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.FuncDecl:
		if d.entry.runDecls[n.Name.Name] == n {
			d.reportRunDecl(n)
		}
	case *ast.CallExpr:
		d.inspectCall(n)
	case *ast.CompositeLit:
		d.inspectComposite(n)
	case *ast.SelectorExpr:
		d.inspectResultExitCode(n)
	}
	return true
}

func (d *removedTwoZeroDetector) inspectCall(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || d.isCovered(call) || d.reportExitFinish(call) || d.reportVerbosePrint(call, sel) {
		return
	}
	if isEvoIdent(sel.X, d.pkg) {
		d.inspectPackageCall(call, sel.Sel.Name)
		return
	}
	switch d.receiverOf(sel.X) {
	case taskReceiver:
		d.inspectTaskCall(call, sel)
	case outputReceiver:
		d.inspectOutputCall(call, sel)
	case failureReceiver:
		d.inspectFailureCall(call, sel)
	}
}

// entryPoints are the removed package-level names that started a 1.x
// program (review_removed_two_zero_entry.go).
var entryPoints = map[string]bool{
	"Init": true, "New": true, "Task": true, "Group": true, "Sequence": true,
	"Main": true, "Print": true, "Printf": true, "Println": true,
}

// inspectPackageCall migrates a removed package-level call: NextCommand and
// the Option funcs.
func (d *removedTwoZeroDetector) inspectPackageCall(call *ast.CallExpr, name string) {
	switch {
	case entryPoints[name]:
		d.inspectEntryCall(call, name)
	case name == "NextCommand":
		d.report(call, "evo.NextCommand was removed in 2.0; a next step is evo.Next(evo.Action{Command: ...})",
			"replace "+d.nodeSrc(call)+" with "+d.nextAction(call))
	case name == "Run" && len(call.Args) == 2:
		d.reportExecuteCall(call, d.pkg+".Run")
	case runMethods[name]:
		d.reportRunMethod(call, name)
	case name == "PolicyFlag" && len(call.Args) == 1:
		d.reportNonInteractive(call)
	case supersededOptionFuncs[name]:
		d.inspectOptionCall(call, name)
	}
}

// nextAction is the 2.0 spelling of an evo.NextCommand(exe, args...) call.
func (d *removedTwoZeroDetector) nextAction(call *ast.CallExpr) string {
	command := "[]string{" + d.callArgsSrc(call.Args) + "}"
	if call.Ellipsis.IsValid() && len(call.Args) > 1 {
		command = "append([]string{" + d.nodeSrc(call.Args[0]) + "}, " + d.nodeSrc(call.Args[len(call.Args)-1]) + "...)"
	}
	return d.pkg + ".Next(" + d.pkg + ".Action{Command: " + command + "})"
}

func (d *removedTwoZeroDetector) isEvoCall(e ast.Expr, name string) (*ast.CallExpr, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return call, ok && sel.Sel.Name == name && isEvoIdent(sel.X, d.pkg)
}

// callArgsSrc is args as written, joined by commas.
func (d *removedTwoZeroDetector) callArgsSrc(args []ast.Expr) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = d.nodeSrc(a)
	}
	return strings.Join(parts, ", ")
}

func (d *removedTwoZeroDetector) report(n ast.Node, message, suggestion string) {
	d.reported = append(d.reported, d.nodeSpan(n))
	d.findings = append(d.findings, Finding{
		RuleID:     "API-032",
		Message:    message,
		File:       d.filename,
		Line:       lineAt(d.src, d.offset(n)),
		Suggestion: oneLine(suggestion),
	})
}

// oneLine joins a suggestion's source lines: a Suggestion is one line, and
// the whitespace between two tokens never matters to a rewrite.
func oneLine(s string) string { return newlineRun.ReplaceAllString(s, " ") }

var newlineRun = regexp.MustCompile(`[ \t]*\n\s*`)

func (d *removedTwoZeroDetector) cover(n ast.Node) { d.covered = append(d.covered, d.nodeSpan(n)) }

func (d *removedTwoZeroDetector) isCovered(n ast.Node) bool {
	off := d.offset(n)
	for _, s := range d.covered {
		if s.contains(off) {
			return true
		}
	}
	return false
}

type srcSpan struct{ start, end int }

func (s srcSpan) contains(offset int) bool { return offset >= s.start && offset < s.end }

func (d *removedTwoZeroDetector) nodeSrc(n ast.Node) string {
	s := d.nodeSpan(n)
	if s.start < 0 || s.end > len(d.src) || s.start >= s.end {
		return ""
	}
	return d.src[s.start:s.end]
}

func (d *removedTwoZeroDetector) nodeSpan(n ast.Node) srcSpan {
	f := d.fset.File(n.Pos())
	return srcSpan{start: f.Offset(n.Pos()), end: f.Offset(n.End())}
}

func (d *removedTwoZeroDetector) offset(n ast.Node) int {
	return d.fset.File(n.Pos()).Offset(n.Pos())
}
