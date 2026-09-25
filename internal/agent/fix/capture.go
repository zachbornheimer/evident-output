package fix

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

// captureRenames indexes retired.CaptureRenames by the removed spelling —
// the one table shared with internal/agent/review and internal/agent/rules
// (E-121), so this analyzer's mapping can never drift from theirs.
var captureRenames = func() map[string]retired.CaptureRename {
	m := make(map[string]retired.CaptureRename, len(retired.CaptureRenames))
	for _, r := range retired.CaptureRenames {
		m[r.From] = r
	}
	return m
}()

// CaptureAnalyzer is API-110..API-116: a call site still spells one of the
// capture-meaning Evidence* names removed in 1.1 (Evidence now means only
// satisfaction proof; retained process output is Capture). It fixes
// selectors on the evo package (evo.Evidence, evo.EvidenceOption, ...);
// (*evo.TaskHandle).Evidence is a method rename with the same From/To
// spelling, resolved through recvNamedType instead of a package selector.
var CaptureAnalyzer = &analysis.Analyzer{
	Name:     "evocapture",
	Doc:      "flags and fixes the capture-meaning Evidence* names removed in 1.1 (E-121)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runCapture,
}

func runCapture(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node) {
		sel := n.(*ast.SelectorExpr)
		rename, removed := captureRenames[sel.Sel.Name]
		if !removed {
			return
		}

		// evo.<Name>: a package-level type/const/func reference. sel.X
		// ("evo") resolves as a valid import use even though the removed
		// member itself does not, so this check does not need sel.Sel to
		// resolve at all — unlike Warn (fully deleted from the package),
		// every capture rename's From name at least used to be a real
		// export, and isEvoPackageSelector only inspects sel.X.
		if isEvoPackageSelector(pass, sel) {
			pass.Report(captureFinding(pass, sel, rename))
			return
		}

		// (*evo.TaskHandle).Evidence(...): the one method spelling among
		// the renames (the rest are package-level types/consts/funcs).
		// recvNamedType likewise only needs sel.X's type, which resolves
		// even when the member itself is unknown.
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); ok && recv == "TaskHandle" {
			pass.Report(captureFinding(pass, sel, rename))
		}
	})
	return nil, nil
}

// isEvoPackageSelector reports whether sel.X is the local import alias for
// the evo package, resolved through the file's own import declarations
// rather than a fixed "evo" identifier check.
func isEvoPackageSelector(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pkgName, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
	return ok && pkgName.Imported().Path() == EvoPackagePath
}

func captureFinding(pass *analysis.Pass, sel *ast.SelectorExpr, rename retired.CaptureRename) analysis.Diagnostic {
	recv := text(pass, sel.X)
	newText := recv + "." + rename.To
	return diag(rename.RuleID, sel,
		recv+"."+rename.From+" was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture",
		analysis.SuggestedFix{
			Message:   "replace " + rename.From + " with " + rename.To,
			TextEdits: []analysis.TextEdit{{Pos: sel.Pos(), End: sel.End(), NewText: []byte(newText)}},
		})
}
