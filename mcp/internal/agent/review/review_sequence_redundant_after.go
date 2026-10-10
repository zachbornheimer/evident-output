// Package review — API-065 (ZYS-1368): After(x) on a step of a Sequence
// where x is an earlier step of that same Sequence. The Sequence already
// makes every step wait for the one before it, so the edge states nothing
// twice. Review never suggests adding an After inside a Sequence.
package review

import (
	"go/ast"
	"go/token"
)

// detectRedundantAfterInSequence is API-065.
func detectRedundantAfterInSequence(filename string, file *ast.File, fset *token.FileSet) []Finding {
	scan := scanHandles(file, evoImportName(file))
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isAfterCall(call) {
			return true
		}
		consumer, ok := parseHandleChain(call)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			if name, redundant := scan.redundantAfterArg(arg, consumer); redundant {
				pos := fset.Position(arg.Pos())
				findings = append(findings, redundantAfterFinding(filename, pos, name, consumer.root))
			}
		}
		return true
	})
	return findings
}

func isAfterCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "After"
}

// redundantAfterArg is the name of an After argument that the consumer's
// Sequence already orders before it.
func (s handleScan) redundantAfterArg(arg ast.Expr, consumer handleChain) (string, bool) {
	id, ok := arg.(*ast.Ident)
	if !ok {
		return "", false
	}
	producer, known := s.handles[id.Name]
	return id.Name, known && s.sequenceOrders(producer, consumer)
}

func redundantAfterFinding(filename string, pos token.Position, name, sequence string) Finding {
	return Finding{
		RuleID:     "API-065",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Message:    "redundant After(" + name + "): " + name + " is an earlier step of Sequence " + sequence + ", which already orders every step after the one before it",
		Suggestion: "delete After(" + name + "); inside a Sequence declaration order is the ordering, so never add an After between its own steps",
	}
}
