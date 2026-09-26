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

// CaptureAnalyzer is API-110..API-117: a call site still spells one of the
// capture-meaning Evidence* names removed in 1.1 (Evidence now means only
// satisfaction proof; retained process output is Capture). It fixes
// selectors on the evo package (evo.Evidence, evo.EvidenceOption, ...) and
// the one struct field rename, Problem.EvidenceTail -> Problem.CaptureTail
// (API-117), resolved through recvNamedType against "Problem" instead of a
// package selector. There is no TaskHandle method rename in this table:
// evo.TaskHandle never had a public Evidence/Capture method to rename.
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

		// p.EvidenceTail (removed in 1.1): the one struct-field spelling
		// among the renames (the rest are package-level types/consts/funcs).
		// evo.Problem is a
		// type alias to internal/core.Problem (types.go), so its receiver's
		// named type resolves to the internal/core package, not the
		// top-level evo package recvNamedType checks against — hence the
		// dedicated isProblemReceiver instead of recvNamedType here.
		//
		// Scoped to API-117 (EvidenceTail) only: Problem.Evidence is a
		// live, unrelated field (satisfaction-proof Attachments, API-110's
		// own name collides with it only by spelling), so matching any
		// rename.From against a Problem receiver would also flag that
		// live field. sel.Sel.Name already equals rename.From here, so
		// this check is exactly "is this the EvidenceTail rename".
		if rename.RuleID == "API-117" && isProblemReceiver(pass.TypesInfo, sel.X) {
			pass.Report(captureFinding(pass, sel, rename))
		}
	})
	return nil, nil
}

// isProblemReceiver reports whether x's type is evo.Problem (or *evo.Problem)
// — the type alias's underlying internal/core.Problem, unwrapping one
// pointer level the same way recvNamedType does.
func isProblemReceiver(info *types.Info, x ast.Expr) bool {
	t := info.TypeOf(x)
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj.Name() != "Problem" || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == EvoPackagePath+"/internal/core"
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
