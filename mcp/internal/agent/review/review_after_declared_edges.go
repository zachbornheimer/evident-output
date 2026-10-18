// Package review — After edges written into a handle's declaration chain
// (`b := seq.Task("b").After(a)`), the form 2.0 code uses most. EVO-DAG-002
// reads them beside the statement form (`b.After(a)`).
package review

import (
	"go/ast"
	"sort"
)

// declaredAfterEdges lists one edge per After target named in the
// declaration chain of a handle variable.
func declaredAfterEdges(file *ast.File) []afterEdge {
	scan := scanHandles(file, evoImportName(file))
	var edges []afterEdge
	for name, h := range scan.handles {
		for _, parent := range h.chain.afters {
			edges = append(edges, afterEdge{child: name, parent: parent, pos: h.chain.pos})
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].pos < edges[j].pos })
	return edges
}
