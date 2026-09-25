// Package review — the removed 1.1 vocabulary (ZYS-1180 freeze): every
// exported name the freeze took out of evo, each with the rule that
// flags it and its rewrite to the canonical form. One table, one pass.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
)

// removedReceiver says which receivers a removedName matches.
type removedReceiver int

const (
	// onPackage is the evo package itself: evo.Name, called or not.
	onPackage removedReceiver = iota
	// onTask is a value certainly holding an evo Task (taskBindings).
	onTask
)

// removedName is one name the freeze removed.
type removedName struct {
	rule    string
	name    string
	on      removedReceiver
	message string
	// rewrite is the suggestion, given the receiver's source and the call's
	// argument sources (nil args for a non-call reference).
	rewrite func(recv string, args []string) string
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
	tasks := newTaskBindings(file, pkg)
	var findings []Finding
	called := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, args, pos, ok := removedCandidate(n)
		if !ok || called[sel] {
			return true
		}
		if args != nil {
			called[sel] = true
		}
		for _, r := range removedNames {
			if r.name != sel.Sel.Name || !r.matches(sel.X, pkg, tasks) {
				continue
			}
			p := fset.Position(pos)
			findings = append(findings, Finding{
				RuleID: r.rule, Message: r.message, File: filename, Line: p.Line, Column: p.Column,
				Suggestion: r.rewrite(types.ExprString(sel.X), args),
			})
		}
		return true
	})
	return findings
}

// removedCandidate is the selector a node uses, with the call's argument
// sources when it is called.
func removedCandidate(n ast.Node) (sel *ast.SelectorExpr, args []string, pos token.Pos, ok bool) {
	switch v := n.(type) {
	case *ast.CallExpr:
		sel, ok = v.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, nil, 0, false
		}
		args = make([]string, len(v.Args), len(v.Args)+1)
		for i, a := range v.Args {
			args[i] = types.ExprString(a)
		}
		return sel, args, v.Pos(), true
	case *ast.SelectorExpr:
		// A bare reference: evo.JSONDocument as a type. A call's own Fun
		// selector is visited after the call and skipped by the caller.
		return v, nil, v.Pos(), true
	}
	return nil, nil, 0, false
}

func (r removedName) matches(x ast.Expr, pkg string, tasks taskBindings) bool {
	switch r.on {
	case onPackage:
		return isEvoIdent(x, pkg)
	case onTask:
		return tasks.IsTask(x)
	}
	return false
}
