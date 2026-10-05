// Package review — API-110..API-116 (E-121): a call site still spells one
// of the capture-meaning Evidence* names removed in 1.1. Evidence now
// means only satisfaction proof; retained process output is Capture.
//
// Detection is structural: a selector <evo>.<Name> on the file's evo
// import, so TaskEvidence/EvidencePhase (canonical satisfaction proof) and
// a same-named identifier from another package stay silent.
package review

import (
	"go/ast"
	"go/token"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

// captureRename is one removed capture-meaning name and its 1.1 spelling.
type captureRename struct {
	ruleID string
	to     string
}

// captureRenames maps each removed Evidence* name to its rule and
// replacement, built from retired.CaptureRenames — the one table shared
// with the MCP rules in internal/agent/rules/rules_capture.go, so the two
// cannot drift.
var captureRenames = func() map[string]captureRename {
	m := make(map[string]captureRename, len(retired.CaptureRenames))
	for _, r := range retired.CaptureRenames {
		m[r.From] = captureRename{ruleID: r.RuleID, to: r.To}
	}
	return m
}()

// detectRemovedCaptureName reports every <evo>.<removed Evidence* name>.
func detectRemovedCaptureName(filename string, file *ast.File, fset *token.FileSet) []Finding {
	evoPkg := evoImportName(file)
	if evoPkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != evoPkg {
			return true
		}
		rename, removed := captureRenames[sel.Sel.Name]
		if !removed {
			return true
		}
		oldName, newName := evoPkg+"."+sel.Sel.Name, evoPkg+"."+rename.to
		pos := fset.Position(sel.Pos())
		findings = append(findings, Finding{
			RuleID:     rename.ruleID,
			Message:    oldName + " was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace " + oldName + " with " + newName,
		})
		return true
	})
	return findings
}
