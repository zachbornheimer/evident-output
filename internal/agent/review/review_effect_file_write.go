// Package review — API-057 (ZYS-932): a filesystem mutator call hidden
// inside an evo.Effect callback. Effect is the opaque-mutation escape hatch
// for work Evo cannot model declaratively (a git ref, a remote API call, a
// database row) — not a second file-write API (ZYS-851's Decisions,
// 2026-09-23). File-backed state must route through evo.File directly.
// This detector always suggests evo.File, even when the callback also reads
// the file it writes; evo.Files (fed by evo.Patch) is the diff-shaped
// alternative and commits through File's own code path.
//
// Detection is structural, per ZYS-851's Decisions: "MCP file-looking
// detection is structural, not based on an object string that merely
// resembles a path. Flag known filesystem mutation calls inside an Effect
// callback (for example os.WriteFile, os.Create, write-mode os.OpenFile,
// os.Remove, os.Rename, and equivalent known filesystem abstractions). A
// path-looking Object alone is not sufficient evidence." A callback passed
// as a same-file named function is resolved to that function's body, not
// just a literal — the mutation is just as hidden either way.
package review

import (
	"go/ast"
	"go/token"
)

// fsWriteMutatorNames are the filesystem mutation calls ZYS-851's Decisions
// names outright, plus ioutil.WriteFile as os.WriteFile's pre-io/fs
// synonym. os.OpenFile is handled separately (isWriteModeOpenFile) because
// only a write-mode flag argument makes it a mutator.
var fsWriteMutatorNames = map[string]bool{
	"os.WriteFile": true, "os.Create": true, "os.Remove": true,
	"os.Rename": true, "ioutil.WriteFile": true,
}

// detectFileWriteInEffectCallback is API-057: an evo.Effect callback
// (literal, or a same-file named function passed as the callback) contains
// a filesystem mutator call.
func detectFileWriteInEffectCallback(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	funcs := fileFuncDeclsByName(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isEvoEffectCall(call, pkg) {
			return true
		}
		body := effectCallbackFuncBody(call.Args[effectCallbackArg], funcs)
		if body == nil {
			return true
		}
		pos, name, ok := firstFSWriteMutator(body)
		if !ok {
			return true
		}
		p := fset.Position(pos)
		findings = append(findings, fileWriteInEffectFinding(filename, p, name))
		return true
	})
	return findings
}

// fileFuncDeclsByName indexes every top-level function declaration in file
// by name, for resolving an Effect callback passed as a bare identifier
// (evo.Effect(ctx, spec, writeConfig)) to the function body that actually
// does the mutating — the same same-file resolution API-042's
// noOpCallbackShape already relies on for its "named" callback shape.
func fileFuncDeclsByName(file *ast.File) map[string]*ast.FuncDecl {
	funcs := map[string]*ast.FuncDecl{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
			funcs[fd.Name.Name] = fd
		}
		return true
	})
	return funcs
}

// effectCallbackFuncBody resolves fn — an evo.Effect callback argument —
// to its function body, whether fn is a literal or a same-file named
// function identifier.
func effectCallbackFuncBody(fn ast.Expr, funcs map[string]*ast.FuncDecl) *ast.BlockStmt {
	switch arg := fn.(type) {
	case *ast.FuncLit:
		return arg.Body
	case *ast.Ident:
		if fd, ok := funcs[arg.Name]; ok {
			return fd.Body
		}
	}
	return nil
}

// firstFSWriteMutator returns the first filesystem mutator call reachable
// anywhere inside body (including nested closures) — a known name from
// fsWriteMutatorNames, or a write-mode os.OpenFile.
func firstFSWriteMutator(body *ast.BlockStmt) (pos token.Pos, name string, found bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if dotted := calledFuncDotted(call); fsWriteMutatorNames[dotted] {
			pos, name, found = call.Pos(), dotted, true
			return false
		}
		if isWriteModeOpenFile(call) {
			pos, name, found = call.Pos(), "os.OpenFile", true
			return false
		}
		return true
	})
	return pos, name, found
}

// isWriteModeOpenFile reports whether call is os.OpenFile(path, flag, perm)
// with a flag argument that names O_WRONLY or O_RDWR — write capability,
// structurally, never a read-only open (os.O_RDONLY alone stays silent).
func isWriteModeOpenFile(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) < 2 {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "os" || sel.Sel.Name != "OpenFile" {
		return false
	}
	return openFileFlagIsWriteMode(call.Args[1])
}

// openFileFlagIsWriteMode reports whether flag's expression tree names
// os.O_WRONLY or os.O_RDWR anywhere (a bare flag, or one side of a bitwise
// OR with os.O_CREATE/os.O_TRUNC/os.O_APPEND).
func openFileFlagIsWriteMode(flag ast.Expr) bool {
	write := false
	ast.Inspect(flag, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "os" {
			return true
		}
		if sel.Sel.Name == "O_WRONLY" || sel.Sel.Name == "O_RDWR" {
			write = true
		}
		return true
	})
	return write
}

// fileWriteInEffectFinding builds API-057's Finding. The Suggestion always
// names evo.File: a write derived from an existing file's contents routes
// through evo.File (read the existing contents first, then pass the
// derived result as FileSpec.Contents). evo.Files commits through the same
// File path, so evo.File is never the wrong suggestion.
func fileWriteInEffectFinding(filename string, pos token.Position, calleeName string) Finding {
	return Finding{
		RuleID:     "API-057",
		Message:    calleeName + " mutates the filesystem directly inside an evo.Effect callback; Effect is the opaque-mutation escape hatch for work Evo cannot model declaratively, not a second file-write API",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "delete the evo.Effect wrapping this file write; call evo.File(ctx, evo.FileSpec{Path: path, Contents: contents}) directly for the desired file state",
	}
}
