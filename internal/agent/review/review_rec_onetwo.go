package review

import "go/ast"

// dialectOneTwo is the first release where File, Tree, and Exec are plain
// structs, Basis is Task freshness, and Patch applies directly (ZYS-1382).
const dialectOneTwo = "1.2.0"

// oneTwoRemoval is one evo export ZYS-1382 retired: why it is gone, and the
// live identifier a selector can be renamed to when the rename is
// one-to-one. An empty next means the call site needs a hand rewrite.
type oneTwoRemoval struct {
	note, next string
}

var oneTwoRemovals = map[string]oneTwoRemoval{
	"FileSpec": {note: "write evo.File{Path, Content, Mode}.Write(ctx)"},
	"ExecSpec": {note: "run evo.Exec{Path, Args, Dir, Env, Outputs}.Run(ctx)"},
	"FSPath":   {note: "declare inputs with task.Basis(evo.File{Path: p}) or evo.Tree{Path: p}"},
	"FileSet":  {note: "apply the diff with evo.Patch(ctx, diff) error"},
	"Files":    {note: "apply the diff with evo.Patch(ctx, diff) error"},
	"ErrStaleBasis": {
		note: "a concurrently edited target is ErrPatchStale", next: "ErrPatchStale",
	},
	"ErrFileSpecMissingPath": {
		note: "a File or Tree without Path is ErrPathMissing", next: "ErrPathMissing",
	},
	"ErrFileUnmanagedContentsMissing": {
		note: "a Write with nil Content is ErrContentMissing", next: "ErrContentMissing",
	},
	"ErrExecSpecMissingExecutable": {
		note: "an Exec without Path is ErrExecPathMissing", next: "ErrExecPathMissing",
	},
	"ErrPatchDeleteUnsupported": {note: "Patch applies deletes; drop the branch that handled this error"},
	"ErrPatchRenameUnsupported": {note: "Patch applies renames; drop the branch that handled this error"},
}

// inspectOneTwoType flags a selector naming an export ZYS-1382 retired.
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
