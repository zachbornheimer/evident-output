// Package review — API-066 (ZYS-1368): a function-level variable that one
// Define callback assigns and a different Define callback reads. That is a
// Task result handed over through mutable outer state, with no compiler
// check and no statement of order; evo.Compute returns the value typed and
// After(computed) states the order (contract §31).
package review

import (
	"go/ast"
	"go/token"
	"slices"
)

// detectOuterVariableHandoff is API-066.
func detectOuterVariableHandoff(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		for _, h := range outerHandoffs(fn.Body, defineCallbacks(file, evoImportName(file))) {
			pos := fset.Position(h.pos)
			findings = append(findings, Finding{
				RuleID:     "API-066",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Message:    h.name + " hands a Task result to another Task through an outer variable: the compiler checks nothing and no edge states the order",
				Suggestion: h.name + " := evo.Compute(producer, func(ctx context.Context) (T, error) { ... }); then the consumer declares After(" + h.name + ") and reads " + h.name + ".Get()",
			})
		}
	}
	return findings
}

type outerHandoff struct {
	name string
	pos  token.Pos
}

// outerHandoffs are the outer variables of body assigned in one callback
// and read in another callback that neither contains nor is contained by it.
func outerHandoffs(body *ast.BlockStmt, all []defineCallback) []outerHandoff {
	var callbacks []*ast.FuncLit
	for _, cb := range all {
		if cb.lit.Pos() >= body.Pos() && cb.lit.End() <= body.End() {
			callbacks = append(callbacks, cb.lit)
		}
	}
	outer := declaredOutside(body, callbacks)
	var found []outerHandoff
	reported := map[string]bool{}
	for _, writer := range callbacks {
		for _, w := range assignedOuter(writer, outer) {
			if !reported[w.name] && readByOtherCallback(w.name, writer, callbacks) {
				reported[w.name] = true
				found = append(found, w)
			}
		}
	}
	return found
}

// declaredOutside is every name body declares outside all callbacks.
func declaredOutside(body *ast.BlockStmt, callbacks []*ast.FuncLit) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, isLit := n.(*ast.FuncLit); isLit && containsLit(callbacks, lit) {
			return false
		}
		for _, name := range declaredNames(n) {
			names[name] = true
		}
		return true
	})
	delete(names, "_")
	delete(names, "err")
	return names
}

func containsLit(lits []*ast.FuncLit, lit *ast.FuncLit) bool {
	return slices.Contains(lits, lit)
}

// declaredNames are the identifiers n declares with := or var.
func declaredNames(n ast.Node) []string {
	var names []string
	switch n := n.(type) {
	case *ast.AssignStmt:
		if n.Tok != token.DEFINE {
			return nil
		}
		for _, lhs := range n.Lhs {
			names = append(names, identName(lhs))
		}
	case *ast.ValueSpec:
		for _, id := range n.Names {
			names = append(names, id.Name)
		}
	}
	return names
}

// assignedOuter are the outer variables lit assigns with plain = that it
// does not redeclare locally.
func assignedOuter(lit *ast.FuncLit, outer map[string]bool) []outerHandoff {
	local := map[string]bool{}
	var assigned []outerHandoff
	inspectOwnBody(lit.Body, func(n ast.Node) {
		for _, name := range declaredNames(n) {
			local[name] = true
		}
		if as, ok := n.(*ast.AssignStmt); ok && as.Tok == token.ASSIGN {
			for _, lhs := range as.Lhs {
				if name := identName(lhs); outer[name] {
					assigned = append(assigned, outerHandoff{name, lhs.Pos()})
				}
			}
		}
	})
	var out []outerHandoff
	for _, a := range assigned {
		if !local[a.name] {
			out = append(out, a)
		}
	}
	return out
}

// readByOtherCallback reports whether any callback unrelated to writer
// mentions name.
func readByOtherCallback(name string, writer *ast.FuncLit, callbacks []*ast.FuncLit) bool {
	for _, reader := range callbacks {
		if reader == writer || nestsEitherWay(reader, writer) {
			continue
		}
		if mentionsIdent(reader, name) {
			return true
		}
	}
	return false
}

func nestsEitherWay(a, b *ast.FuncLit) bool {
	return (a.Pos() <= b.Pos() && b.End() <= a.End()) || (b.Pos() <= a.Pos() && a.End() <= b.End())
}

func mentionsIdent(lit *ast.FuncLit, name string) bool {
	found := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}
