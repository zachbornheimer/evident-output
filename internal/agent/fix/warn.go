package fix

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// evoLocalsCache memoizes evoValuedIdents per file within one analyzer
// run, since several calls in the same file each need it.
type evoLocalsCache map[*ast.File]map[string]bool

func (c evoLocalsCache) get(f *ast.File, alias string) map[string]bool {
	if locals, ok := c[f]; ok {
		return locals
	}
	locals := evoValuedIdents(f, alias)
	c[f] = locals
	return locals
}

// WarnAnalyzer is API-070: Warn was removed in 1.1 — Problem wins over
// Warn, warning is a Problem severity. (*evo.TaskHandle).Warn has a
// mechanical rewrite to .Problem(..., evo.Severity(evo.SeverityWarning)).
// evo.Output.Warn and the package-level evo.Warn have no Task to attach
// the Problem to, so attaching one is not mechanical: those report the
// removal with no SuggestedFix and name the manual step.
var WarnAnalyzer = &analysis.Analyzer{
	Name:     "evowarn",
	Doc:      "flags evo Warn calls removed in 1.1 (API-070) and fixes the TaskHandle case",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runWarn,
}

func runWarn(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	locals := evoLocalsCache{}
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil || sel.Sel.Name != "Warn" {
			return true
		}

		// The typed path: fires when the file still type-checks around
		// this call (a receiver that resolves to evo.TaskHandle/Output,
		// or the package-level evo.Warn — removed in 1.1 — as a still-resolvable symbol).
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); ok {
			reportWarn(pass, call, sel, recv == "TaskHandle")
			return true
		}
		if _, ok := packageFunc(pass.TypesInfo, sel); ok {
			reportWarn(pass, call, sel, false)
			return true
		}

		// Warn was fully removed from evo (not merely deprecated), so a
		// live call site is a hard compile error: go/types has no Uses
		// entry to resolve, and the typed path above can never fire.
		// Fall back to import-alias/evo-local tracing, same as
		// internal/agent/review's evoWarn detector for the same reason.
		f := enclosingFile(pass, call.Pos())
		if f == nil {
			return true
		}
		alias := evoImportAlias(pass, call)
		if alias == "" {
			return true
		}
		known := locals.get(f, alias)
		if !aliasReceiver(alias, sel.X, known) {
			return true
		}
		isTask := isLikelyTaskReceiver(sel.X, alias, known, stack)
		reportWarn(pass, call, sel, isTask)
		return true
	})
	return nil, nil
}

func reportWarn(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, isTask bool) {
	if isTask {
		pass.Report(diag("API-070", call,
			"(*evo.TaskHandle).Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity",
			warnTaskFix(pass, call, sel)))
		return
	}
	pass.Report(diag("API-070", call,
		"evo.Warn was removed in 1.1: declare a Task and call its Problem(summary, evo.Severity(evo.SeverityWarning)) instead — attaching a run-scoped warning to a Task is not mechanical"))
}

// isLikelyTaskReceiver distinguishes a Task receiver from Output/package
// receiver when only structural tracing is available (the removed-name
// fallback): the bare import alias itself is never a Task; everything
// else known evo-valued (a Group/Task chain result, a traced local) is
// treated as a Task, since Output.Warn (also removed in 1.1) call sites
// are rare and the worst case is an offered TaskHandle fix the reader declines.
func isLikelyTaskReceiver(x ast.Expr, alias string, known map[string]bool, _ []ast.Node) bool {
	if id, ok := x.(*ast.Ident); ok && id.Name == alias {
		return false
	}
	return true
}

// warnTaskFix rewrites task.Warn(summary, opts...) (removed in 1.1) to
// task.Problem(summary, append(opts, evo.Severity(evo.SeverityWarning))...).
// When there are no extra ProblemOptions the append is unnecessary, so the
// fix writes the simpler task.Problem(summary, evo.Severity(evo.SeverityWarning)).
func warnTaskFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr) analysis.SuggestedFix {
	alias := evoAlias(pass, sel)
	replacement := "Problem"
	edits := []analysis.TextEdit{{
		Pos:     sel.Sel.Pos(),
		End:     sel.Sel.End(),
		NewText: []byte(replacement),
	}}
	if len(call.Args) > 0 {
		last := call.Args[len(call.Args)-1]
		severity := alias + ".Severity(" + alias + ".SeverityWarning)"
		edits = append(edits, analysis.TextEdit{
			Pos:     last.End(),
			End:     last.End(),
			NewText: []byte(", " + severity),
		})
	}
	return analysis.SuggestedFix{
		Message:   "replace Warn with Problem(..., evo.Severity(evo.SeverityWarning))",
		TextEdits: edits,
	}
}

// evoAlias returns the local import alias for the evo package in the file
// that contains sel, defaulting to "evo" when it cannot be determined
// (e.g. the receiver is a local variable, not the package identifier).
func evoAlias(pass *analysis.Pass, sel *ast.SelectorExpr) string {
	f := enclosingFile(pass, sel.Pos())
	if f == nil {
		return "evo"
	}
	for _, imp := range f.Imports {
		if importPath(imp) != EvoPackagePath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "evo"
	}
	return "evo"
}

func importPath(imp *ast.ImportSpec) string {
	if imp.Path == nil {
		return ""
	}
	v := imp.Path.Value
	if len(v) >= 2 {
		return v[1 : len(v)-1]
	}
	return v
}
