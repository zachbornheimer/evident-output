package review

import (
	"go/ast"
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// removedExports are the package-level names 1.1 and 1.2 removed, keyed by
// name, each with the retired table's 2.0 replacement. The table is the one
// owner of what replaces a name; this arm only reports a use of it. Method
// forms (a Contract with "." or "(") belong to the receiver arms, and 1.0 and
// 2.0 removals have their own arms or detectors.
var removedExports = sync.OnceValue(func() map[string]rules.RetiredSymbol {
	exports := map[string]rules.RetiredSymbol{}
	for _, s := range rules.RetiredSymbols() {
		if strings.ContainsAny(s.Contract, ".(") {
			continue
		}
		if s.RemovedIn == rules.RetiredRelease1_1 || s.RemovedIn == rules.RetiredRelease1_2 {
			exports[s.Contract] = s
		}
	}
	return exports
})

// inspectRemovedExport reports a use of a removed export that no more
// specific arm already migrated.
func (d *removedTwoZeroDetector) inspectRemovedExport(n ast.Node) bool {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, d.pkg) || d.isCovered(sel) {
		return true
	}
	removed, known := removedExports()[sel.Sel.Name]
	if !known {
		return true
	}
	d.report(sel, "evo."+removed.Contract+" was removed in "+string(removed.RemovedIn)+"; "+removed.Replacement,
		"instead: "+removed.Replacement)
	d.cover(sel)
	return true
}
