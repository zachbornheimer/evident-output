// Package review — API-064 (ZYS-1368): Computed.Get() read by work that
// nothing orders after the producing Task. Get is valid only once the
// producer has succeeded (contract §31), and After(computed) is the one
// edge that states both the order and the data dependency. A later step of
// the producer's own Sequence is already ordered, and so is a reader nested
// inside an ordered builder, so neither is flagged.
package review

import (
	"go/ast"
	"go/token"
)

// detectUnorderedComputedGet is API-064.
func detectUnorderedComputedGet(filename string, file *ast.File, fset *token.FileSet) []Finding {
	evoPkg := evoImportName(file)
	scan := scanHandles(file, evoPkg)
	consumers := scan.consumers(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		producer, ok := scan.computedGetReceiver(call)
		if !ok {
			return true
		}
		enclosing := consumersAround(consumers, call.Pos())
		if len(enclosing) == 0 || anyOrderedAfter(scan, enclosing, producer) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, unorderedGetFinding(filename, pos, producer.name))
		return true
	})
	return findings
}

// computedGetReceiver is the Compute result x in a bare x.Get() call.
func (s handleScan) computedGetReceiver(call *ast.CallExpr) (declaredHandle, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Get" || len(call.Args) != 0 {
		return declaredHandle{}, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return declaredHandle{}, false
	}
	h, known := s.handles[id.Name]
	return h, known && h.computed
}

// consumersAround are the callbacks whose body contains pos.
func consumersAround(all []consumer, pos token.Pos) []consumer {
	var around []consumer
	for _, c := range all {
		if c.lit.Pos() <= pos && pos < c.lit.End() {
			around = append(around, c)
		}
	}
	return around
}

func anyOrderedAfter(s handleScan, consumers []consumer, producer declaredHandle) bool {
	for _, c := range consumers {
		if s.waitsOn(c.chain, producer.name) || s.sequenceOrders(producer, c.chain) {
			return true
		}
	}
	return false
}

func unorderedGetFinding(filename string, pos token.Position, name string) Finding {
	return Finding{
		RuleID:     "API-064",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Message:    name + ".Get() has no structural ordering: nothing guarantees the producing Task succeeded before this work reads it, and early access is deterministic misuse",
		Suggestion: "add After(" + name + ") to the Task or Group that reads it, e.g. container.Task(...).After(" + name + ").Define(...)",
	}
}
