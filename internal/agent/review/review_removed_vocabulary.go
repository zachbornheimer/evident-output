// Package review — the removed 1.1 vocabulary (ZYS-1180 freeze): every
// exported name the freeze took out of evo, each with the rule that
// flags it and its rewrite to the canonical form. One table, one pass.
package review

import (
	"go/ast"
	"go/token"
)

// removedName is one name the freeze removed. Every entry so far is a
// package-level export (evo.Name, called or not); add a receiver kind here
// only when a removed name needs one.
type removedName struct {
	rule       string
	name       string
	message    string
	suggestion string
}

// removedNames is the table. Adding a removed name adds one entry.
var removedNames []removedName

// registerRemoved adds entries to removedNames at init, so each freeze
// item's entries live beside the rationale for that item.
func registerRemoved(entries ...removedName) { removedNames = append(removedNames, entries...) }

// detectRemovedVocabulary flags every use of a removed name.
func detectRemovedVocabulary(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	called := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := removedCandidate(n)
		if !ok || called[sel] {
			return true
		}
		if _, isCall := n.(*ast.CallExpr); isCall {
			called[sel] = true
		}
		for _, r := range removedNames {
			if r.name != sel.Sel.Name || !isEvoIdent(sel.X, pkg) {
				continue
			}
			p := fset.Position(sel.Pos())
			findings = append(findings, Finding{
				RuleID: r.rule, Message: r.message, File: filename, Line: p.Line, Column: p.Column,
				Suggestion: r.suggestion,
			})
		}
		return true
	})
	return findings
}

// removedCandidate is the selector a node uses: a call (evo.ForSkip(),
// removed in 1.1) or a bare reference (evo.ReasonOption, removed in 1.1,
// as a type).
func removedCandidate(n ast.Node) (*ast.SelectorExpr, bool) {
	switch v := n.(type) {
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		return sel, ok
	case *ast.SelectorExpr:
		return v, true
	}
	return nil, false
}
