package review

import "go/ast"

// dialectOneTwo is the first release where Tree/Find/Download/Extract/Clone/
// Checksum are not evo (contract §31/§37; ZYS-1382 is not 1.2). File/Patch/
// Exec stay the 1.1 operations.
const dialectOneTwo = "1.2.0"

// oneTwoRemoval is one evo export 1.2 retired: why it is gone, and the live
// identifier a selector can be renamed to when the rename is one-to-one.
// An empty next means the call site needs a hand rewrite.
type oneTwoRemoval struct {
	note, next string
}

var oneTwoRemovals = map[string]oneTwoRemoval{
	"Tree":     {note: "caller-owned tree work, then evo.File / evo.Effect"},
	"Find":     {note: "caller-owned discovery, then evo.File"},
	"Download": {note: "caller fetch, then evo.File"},
	"Extract":  {note: "caller unpack, then evo.File"},
	"Clone":    {note: "caller copy, then evo.File"},
	"Checksum": {note: "not evo"},
}

// inspectOneTwoType flags a selector naming an export 1.2 retired.
func (d *recSurfaceDetector) inspectOneTwoType(n ast.Node) bool {
	if !d.fileTreeDialect {
		return false
	}
	sel, ok := n.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, d.pkg) {
		return false
	}
	removal, known := oneTwoRemovals[sel.Sel.Name]
	if !known || d.isCovered(sel) {
		return false
	}
	sug := ""
	if removal.next != "" {
		sug = "replace " + d.nodeSrc(sel) + " with " + d.pkg + "." + removal.next
	}
	d.report(sel, sel.Sel.Name+" was removed in 1.2; "+removal.note, sug)
	d.cover(sel)
	return true
}
