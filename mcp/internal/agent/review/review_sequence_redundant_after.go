// Package review — API-065 (ZYS-1368): After(x) on a step of a Sequence
// where x is an earlier step of that same Sequence. The Sequence already
// makes every step wait for the one before it, so the edge states nothing
// twice. Review never suggests adding an After inside a Sequence.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
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
		edit := redundantAfterEdit(scan, call, consumer)
		for _, arg := range call.Args {
			if name, redundant := scan.redundantAfterArg(arg, consumer); redundant {
				pos := fset.Position(arg.Pos())
				findings = append(findings, redundantAfterFinding(filename, pos, name, consumer.root, edit))
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

const sequenceOrdersWhy = "; inside a Sequence declaration order is the ordering, so never add an After between its own steps"

// redundantAfterEdit is the exact edit that removes every redundant argument
// of call: the whole selector call, dot included, when none is left, else the
// call with only the arguments the Sequence does not already order. The
// replace form ends at its new text, so only the delete form carries the why.
func redundantAfterEdit(scan handleScan, call *ast.CallExpr, consumer handleChain) string {
	var kept []string
	for _, arg := range call.Args {
		if _, redundant := scan.redundantAfterArg(arg, consumer); !redundant {
			kept = append(kept, types.ExprString(arg))
		}
	}
	old := ".After(" + exprList(call.Args) + ")"
	if len(kept) == 0 {
		return "delete " + old + sequenceOrdersWhy
	}
	return "replace " + old + " with .After(" + strings.Join(kept, ", ") + ")"
}

func exprList(args []ast.Expr) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = types.ExprString(a)
	}
	return strings.Join(parts, ", ")
}

func redundantAfterFinding(filename string, pos token.Position, name, sequence, edit string) Finding {
	return Finding{
		RuleID:     "API-065",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Message:    "redundant After(" + name + "): " + name + " is an earlier step of Sequence " + sequence + ", which already orders every step after the one before it",
		Suggestion: edit,
	}
}
