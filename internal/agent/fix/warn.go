package fix

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// WarnAnalyzer is API-070: Warn was removed in 1.1 — Problem wins over
// Warn, warning is a Problem severity. (*evo.TaskHandle).Warn has a
// mechanical rewrite to .Problem(..., evo.Severity(evo.SeverityWarning)).
// evo.Output.Warn and the package-level evo.Warn have no Task to attach
// the Problem to, so attaching one is not mechanical: those report the
// removal with no SuggestedFix and name the manual step.
//
// Detection is purely typed: a method call resolves the receiver
// expression's own type (recvNamedType), never the removed Warn selector
// itself. That works even though Warn no longer exists on any evo type —
// go/types still records a valid type for the receiver expression (e.g.
// the *evo.TaskHandle a prior assignment declared) independent of whether
// the method call built on top of it type-checks. The package-level
// evo.Warn case resolves the same way: isEvoPackageSelector only needs
// sel.X (the "evo" identifier) to resolve to the evo package's PkgName,
// which go/types still records even though Sel itself — the removed
// Warn — has no Use. No identifier-spelling or import-alias tracing is
// needed to find any of these call sites.
var WarnAnalyzer = &analysis.Analyzer{
	Name:     "evowarn",
	Doc:      "flags evo Warn calls removed in 1.1 (API-070) and fixes the TaskHandle case",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runWarn,
}

func runWarn(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil || sel.Sel.Name != "Warn" {
			return
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); ok {
			reportWarn(pass, call, sel, recv)
			return
		}
		if isEvoPackageSelector(pass, sel) {
			reportWarn(pass, call, sel, "")
		}
	})
	return nil, nil
}

// reportWarn reports the API-070 removal for a Warn call whose receiver
// resolved to the evo package type named recv — "TaskHandle" for a
// mechanical Problem(...) rewrite, any other evo type (e.g. "GroupHandle")
// for a named-but-unfixable removal, or "" for the package-level evo.Warn
// (removed in 1.1).
func reportWarn(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, recv string) {
	switch recv {
	case "TaskHandle":
		if call.Ellipsis != token.NoPos {
			// A spread trailing arg (Warn(s, opts...)) can't take a
			// mechanical ", evo.Severity(...)" append after it without
			// producing a second variadic spread, which does not
			// compile. Leave it for a manual rewrite.
			pass.Report(diag("API-070", call,
				"(*evo.TaskHandle).Warn was removed in 1.1: rewrite to Problem(summary, append(opts, evo.Severity(evo.SeverityWarning))...) by hand — the spread trailing argument isn't a mechanical rewrite"))
			return
		}
		pass.Report(diag("API-070", call,
			"(*evo.TaskHandle).Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity",
			warnTaskFix(pass, call, sel)))
	case "":
		pass.Report(diag("API-070", call,
			"evo.Warn was removed in 1.1: declare a Task and call its Problem(summary, evo.Severity(evo.SeverityWarning)) instead — attaching a run-scoped warning to a Task is not mechanical"))
	default:
		pass.Report(diag("API-070", call,
			"(*evo."+recv+").Warn was removed in 1.1: "+recv+" has no Task to attach a Problem to — declare a Task and call its Problem(summary, evo.Severity(evo.SeverityWarning)) instead"))
	}
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
	if imp := addEvoImport(pass, sel.Pos()); imp.NewText != nil {
		edits = append(edits, imp)
	}
	return analysis.SuggestedFix{
		Message:   "replace Warn with Problem(..., evo.Severity(evo.SeverityWarning))",
		TextEdits: edits,
	}
}

// addEvoImport inserts `evo "github.com/zachbornheimer/evident-output"`
// right after the package clause when the file has no evo import at all.
// warnTaskFix writes literal `evo.Severity(evo.SeverityWarning)` text
// (evoAlias defaults to "evo" when it cannot find an existing import), so
// a package-level evo.Warn (removed in 1.1) call reached via a dot-import
// — one with no named evo import to resolve an alias from — needs the
// import added or the fix leaves `undefined: evo` behind.
func addEvoImport(pass *analysis.Pass, at token.Pos) analysis.TextEdit {
	f := enclosingFile(pass, at)
	if f == nil {
		return analysis.TextEdit{}
	}
	for _, imp := range f.Imports {
		if importPath(imp) == EvoPackagePath {
			return analysis.TextEdit{}
		}
	}
	return analysis.TextEdit{
		Pos:     f.Name.End(),
		End:     f.Name.End(),
		NewText: []byte("\n\nimport evo \"" + EvoPackagePath + "\""),
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
