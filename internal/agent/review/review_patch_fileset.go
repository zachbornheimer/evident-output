// Package review — API-058 (ZYS-935): a Patch-derived FileSet is never
// passed to evo.Files, and the same function commits a freshly built
// evo.FileSpec through evo.File instead. FileSet is opaque specifically so
// its source Basis/stale-write guard cannot be stripped before commit
// (ZYS-841's Decisions, 2026-09-23) — a caller who re-derives the desired
// contents another way and calls evo.File directly reconstructs exactly the
// state Patch already derived, minus the guard against overwriting a source
// that changed after the diff was applied. The remediation is always the
// same: commit through evo.Files(ctx, <the FileSet Patch returned>).
//
// Detection is per-function-body and structural, mirroring API-057: a
// function that calls evo.Patch, never threads that FileSet into an
// evo.Files call in the same body, and also calls evo.File somewhere in
// that body is flagged at each such evo.File call. A FileSet returned to a
// caller that itself commits it through evo.Files elsewhere is not visible
// to this per-body check and stays silent, by design — the same scope
// limitation API-057's per-body Effect/File check already carries.
package review

import (
	"fmt"
	"go/ast"
	"go/token"
)

// detectPatchFileSetDiscardedBeforeCommit is API-058.
func detectPatchFileSetDiscardedBeforeCommit(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		unconsumed, blankDiscard := patchDerivedFileSetVars(body, pkg)
		if len(unconsumed) == 0 && !blankDiscard {
			return
		}

		name := ""
		for n := range unconsumed {
			name = n
			break
		}

		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || calledFuncDotted(call) != pkg+".File" {
				return true
			}
			findings = append(findings, patchFileSetDiscardedFinding(filename, fset.Position(call.Pos()), name))
			return true
		})
	})
	return findings
}

// patchDerivedFileSetVars scans body for every evo.Patch(...) call assigned
// to a named variable, then removes from that set any name later passed as
// an argument to evo.Files(...) anywhere in body. It also reports whether
// any evo.Patch call assigned its FileSet straight to "_" — an immediate
// discard with no variable to ever thread through evo.Files.
func patchDerivedFileSetVars(body *ast.BlockStmt, pkg string) (unconsumed map[string]bool, blankDiscard bool) {
	unconsumed = map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if stmt, ok := n.(*ast.AssignStmt); ok {
			if name, blank, ok := patchAssignTarget(stmt, pkg); ok {
				if blank {
					blankDiscard = true
				} else {
					unconsumed[name] = true
				}
			}
		}
		if call, ok := n.(*ast.CallExpr); ok && calledFuncDotted(call) == pkg+".Files" {
			for _, arg := range call.Args {
				if id, ok := arg.(*ast.Ident); ok {
					delete(unconsumed, id.Name)
				}
			}
		}
		return true
	})
	return unconsumed, blankDiscard
}

// patchAssignTarget reports whether stmt assigns the result of a
// pkg.Patch(...) call to a single identifier target (name, blank=false), to
// "_" (blank=true), or is unrelated (ok=false). A multi-value assignment
// such as "fileSet, err := evo.Patch(ctx, diff)" has one Rhs expression
// (the call) and two Lhs targets; the FileSet lands in Lhs[0].
func patchAssignTarget(stmt *ast.AssignStmt, pkg string) (name string, blank bool, ok bool) {
	if len(stmt.Rhs) != 1 || len(stmt.Lhs) == 0 {
		return "", false, false
	}
	call, isCall := stmt.Rhs[0].(*ast.CallExpr)
	if !isCall || calledFuncDotted(call) != pkg+".Patch" {
		return "", false, false
	}
	id, isIdent := stmt.Lhs[0].(*ast.Ident)
	if !isIdent {
		return "", false, false
	}
	if id.Name == "_" {
		return "", true, true
	}
	return id.Name, false, true
}

// patchFileSetDiscardedFinding builds API-058's Finding. varName is empty
// when the FileSet was discarded straight to "_", in which case the
// Suggestion names the general shape instead of a specific identifier.
func patchFileSetDiscardedFinding(filename string, pos token.Position, varName string) Finding {
	subject := "the FileSet evo.Patch returned"
	suggestion := "commit through evo.Files(ctx, <the FileSet evo.Patch returned>) instead of building a new FileSpec"
	if varName != "" {
		subject = varName
		suggestion = fmt.Sprintf("commit through evo.Files(ctx, %s) instead of building a new FileSpec", varName)
	}
	return Finding{
		RuleID:          "API-058",
		Severity:        "error",
		Message:         fmt.Sprintf("%s is never passed to evo.Files; this evo.File call reconstructs a fresh FileSpec instead, discarding the Patch-derived source Basis and stale-write guard", subject),
		File:            filename,
		Line:            pos.Line,
		Column:          pos.Column,
		Suggestion:      suggestion,
		RequiredVersion: dialectOneOne,
	}
}
