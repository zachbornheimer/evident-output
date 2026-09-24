// Package review — API-062 (ZYS-946, spec §53): a package-level evo
// declaration (evo.Task, evo.Group, evo.Sequence, evo.Fact, evo.Warn,
// evo.Print*, evo.Confirm) reached from the Run callback of an Isolated
// Output. Package-level calls always reach the package default, never the
// Output being run, so under an HTTP server every concurrent request would
// share that one default's state while its own document came back empty.
// The fix is the Output-bound spelling: out.Task(...), or pass the
// *evo.Output into the shared model function.
//
// Detection is structural, without type information. The Output side is
// in review_isolated_outputs.go; following the callback into the
// package's own functions is in review_isolated_callees.go. GoSource
// follows functions declared in the same file; GoDirectory follows them
// across every file of the package.
package review

import (
	"go/ast"
	"go/token"
)

// packageDeclarationFacades are the package-level evo functions that act
// on the package default Output and have an Output-bound method twin.
var packageDeclarationFacades = map[string]bool{
	"Task": true, "Group": true, "Sequence": true, "Fact": true, "Warn": true,
	"Print": true, "Printf": true, "Println": true, "Confirm": true,
}

// goSource is one parsed file of the package API-062 reviews.
type goSource struct {
	name string
	file *ast.File
	// pkg is this file's name for the evo import; empty when the file
	// does not import evo.
	pkg string
}

func newGoSource(name string, file *ast.File) goSource {
	return goSource{name: name, file: file, pkg: evoImportName(file)}
}

// detectPackageFacadeInIsolatedRun is API-062 over one file.
func detectPackageFacadeInIsolatedRun(filename string, file *ast.File, fset *token.FileSet) []Finding {
	return detectPackageFacadeInIsolatedRuns(fset, []goSource{newGoSource(filename, file)})
}

// detectPackageFacadeInIsolatedRuns is API-062 over the files of one
// package: every Run on an Isolated Output, followed into the package's
// own functions, reports each package-level declaration it reaches.
func detectPackageFacadeInIsolatedRuns(fset *token.FileSet, files []goSource) []Finding {
	pkg := newIsolatedPackage(fset, files)
	var findings []Finding
	for _, run := range pkg.isolatedRuns() {
		findings = append(findings, pkg.facadesReachedBy(run)...)
	}
	return findings
}

// facadesReachedBy reports every package-level declaration run's callback
// reaches, directly or through the package's own functions.
func (p *isolatedPackage) facadesReachedBy(run isolatedRun) []Finding {
	var findings []Finding
	p.reach(run, func(at reachedCall) {
		if fn, ok := packageFacadeCall(at.call, at.src.pkg); ok {
			findings = append(findings, packageFacadeInIsolatedRunFinding(at, p.fset.Position(at.call.Pos()), fn, run.output))
		}
	})
	return findings
}

// packageFacadeCall reports the facade name when call is pkg.<facade>(...).
func packageFacadeCall(call *ast.CallExpr, pkg string) (string, bool) {
	if pkg == "" {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, pkg) || !packageDeclarationFacades[sel.Sel.Name] {
		return "", false
	}
	return sel.Sel.Name, true
}

func packageFacadeInIsolatedRunFinding(at reachedCall, pos token.Position, fn, output string) Finding {
	pkg := at.src.pkg
	where := "inside " + output + ".Run"
	suggestion := "call " + output + "." + fn + "(...) instead, or pass " + output + " (*" + pkg + ".Output) into the shared model function so the CLI and HTTP paths declare on whichever Output drives them"
	if at.via != "" {
		where = "in " + at.via + ", which " + output + ".Run reaches,"
		suggestion = "give " + at.via + " a *" + pkg + ".Output parameter, pass " + output + " to it, and call that Output's " + fn + " method; the CLI passes " + pkg + ".Default()"
	}
	return Finding{
		RuleID:   "API-062",
		Severity: "error",
		Message: pkg + "." + fn + " " + where + " declares on the package default, not on the Isolated Output being run — " +
			"concurrent runs share that default and this run's own document omits the work",
		File:            at.src.name,
		Line:            pos.Line,
		Column:          pos.Column,
		Suggestion:      suggestion,
		RequiredVersion: dialectOneTwo,
	}
}
