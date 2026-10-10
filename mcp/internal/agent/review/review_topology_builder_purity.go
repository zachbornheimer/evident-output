// Package review — API-069 (ZYS-1368): a topology builder
// (`Define(func(*evo.GroupHandle))`) that does more than declare structure.
// A builder runs on the scheduler after its predecessor is ready; I/O,
// mutation, context use, goroutines, Wait, or an error return inside it
// start a second scheduler or hide work Evo cannot render (contract §31).
// Work callbacks nested in the builder are not the builder and are skipped.
package review

import (
	"go/ast"
	"go/token"
)

// builderIOPackages are import paths whose calls are I/O or mutation.
var builderIOPackages = []string{"os", "io", "io/ioutil", "io/fs", "os/exec", "net", "net/http", "database/sql"}

// builderWalkCalls are filesystem-walking path/filepath functions.
var builderWalkCalls = map[string]bool{"Walk": true, "WalkDir": true, "Glob": true}

// detectImpureTopologyBuilder is API-069.
func detectImpureTopologyBuilder(filename string, file *ast.File, fset *token.FileSet) []Finding {
	evoPkg := evoImportName(file)
	ioNames := importedNames(file, builderIOPackages)
	filepathName := importNameFor(file, "path/filepath")
	var findings []Finding
	for _, cb := range defineCallbacks(file, evoPkg) {
		if !cb.builder {
			continue
		}
		for _, v := range builderViolations(cb.lit, evoPkg, ioNames, filepathName) {
			pos := fset.Position(v.pos)
			findings = append(findings, Finding{
				RuleID:     "API-069",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Message:    v.what + " inside a topology builder: a builder declares structure only and must not do work or wait",
				Suggestion: "move the work into a predecessor Task exposed with evo.Compute, then read its Get() in the builder after After(computed); the builder only declares children",
			})
		}
	}
	return findings
}

type builderViolation struct {
	pos  token.Pos
	what string
}

func builderViolations(lit *ast.FuncLit, evoPkg string, ioNames map[string]bool, filepathName string) []builderViolation {
	var out []builderViolation
	if lit.Type.Results != nil && len(lit.Type.Results.List) > 0 {
		out = append(out, builderViolation{lit.Pos(), "a returned error"})
	}
	inspectOwnBody(lit.Body, func(n ast.Node) {
		if what := builderViolationOf(n, evoPkg, ioNames, filepathName); what != "" {
			out = append(out, builderViolation{n.Pos(), what})
		}
	})
	return out
}

func builderViolationOf(n ast.Node, evoPkg string, ioNames map[string]bool, filepathName string) string {
	switch n := n.(type) {
	case *ast.GoStmt:
		return "a goroutine"
	case *ast.Ident:
		if n.Name == "ctx" {
			return "context use"
		}
	case *ast.CallExpr:
		return builderCallViolation(n, evoPkg, ioNames, filepathName)
	}
	return ""
}

func builderCallViolation(call *ast.CallExpr, evoPkg string, ioNames map[string]bool, filepathName string) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg := exprDottedName(sel.X)
	switch {
	case sel.Sel.Name == "Wait" && len(call.Args) == 0:
		return "a Wait"
	case pkg == "context":
		return "context use"
	case pkg == evoPkg && (evoManagedCallName(call, evoPkg) != "" || sel.Sel.Name == "Exec"):
		return "I/O (evo." + sel.Sel.Name + ")"
	case ioNames[pkg], pkg == filepathName && pkg != "" && builderWalkCalls[sel.Sel.Name]:
		return "I/O (" + pkg + "." + sel.Sel.Name + ")"
	}
	return ""
}

// importedNames is the set of local names file imports paths under.
func importedNames(file *ast.File, paths []string) map[string]bool {
	names := map[string]bool{}
	for _, p := range paths {
		if name := importNameFor(file, p); name != "" {
			names[name] = true
		}
	}
	return names
}
