// Package review — API-055 (ZYS-931, ZYS-831): a caller-managed lock held
// around an evo.File/Files/Patch/Effect call. The lock is a sync.Mutex or
// RWMutex, a flock(2) on a file descriptor, an O_EXCL lock file removed
// afterwards, or a gofrs/flock value. File claims its own path for writing
// automatically (ZYS-840), so a lock that guards only the Evo call is
// redundant. A lock that also guards other shared state is real and must
// stay; holding it across the call is still worth a warning, because the call
// can wait on a resource claim while the caller's lock is held, a wait Evo
// cannot see or render.
//
// Detection walks the AST: an acquisition statement opens a region that runs
// to its matching bare release (or to the end of the block under a deferred
// release), and the Evo call is matched through the file's own evo import
// name. Comments and strings can never match. Lock-file matchers live in
// review_file_lock.go.
package review

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// evoManagedCalls are the evo functions that own a path or a resource claim.
var evoManagedCalls = []string{"File", "Files", "Patch", "Effect"}

// lockRegion is one acquire…release critical section around an evo call.
type lockRegion struct {
	holder      string // what is held: "w.mu", "fd", the lock-file path
	lockText    string // e.g. "w.mu.Lock()"
	unlockText  string // e.g. "w.mu.Unlock()"
	evoCall     string // e.g. "evo.File"
	pos         token.Pos
	guardsOther bool // the region also does work besides the evo call
}

func detectManualLockAroundEvoFile(filename string, file *ast.File, fset *token.FileSet) []Finding {
	evoPkg := evoImportName(file)
	if evoPkg == "" {
		return nil
	}
	scan := newLockScan(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for _, region := range scan.regions(block.List, evoPkg) {
			findings = append(findings, manualLockFinding(filename, fset.Position(region.pos), region))
		}
		return true
	})
	return findings
}

// manualLockFinding words API-055 for region: removal only when the lock
// guards nothing but the evo call, narrowing otherwise. It never suggests
// deleting the mutex field, which may guard other methods.
func manualLockFinding(filename string, pos token.Position, r lockRegion) Finding {
	pair := r.lockText + "/" + r.unlockText
	f := Finding{
		RuleID: "API-055",
		File:   filename,
		Line:   pos.Line,
		Column: pos.Column,
	}
	if r.guardsOther {
		f.Message = r.holder + " is held across an " + r.evoCall + " call; it may wait on its own resource claim while the caller's lock is held, a wait Evo cannot see"
		f.Suggestion = "keep " + r.holder + " for the shared state it guards, but end the critical section (" + r.unlockText + ") before calling " + r.evoCall
		return f
	}
	f.Message = r.holder + " locks around nothing but an " + r.evoCall + " call; Evo already claims its own path for writing with no caller code"
	f.Suggestion = "drop the " + pair + " pair around this " + r.evoCall + " call; Evo claims its path itself." +
		" For a non-File operation over the same path use evo.Effect(ctx, evo.EffectSpec{..., Resource: evo.FSResource(path)}, fn)"
	return f
}

// regions finds, among one block's statements, every lock region that
// contains an evo call.
func (s lockScan) regions(stmts []ast.Stmt, evoPkg string) []lockRegion {
	var regions []lockRegion
	for i, stmt := range stmts {
		acq, ok := s.acquisition(stmt)
		if !ok {
			continue
		}
		body, released := regionBody(stmt, stmts[i+1:], acq)
		if acq.needsRelease && !released {
			continue
		}
		call := firstEvoManagedCall(body, evoPkg)
		if call == "" {
			continue
		}
		regions = append(regions, lockRegion{
			holder:      acq.holder,
			lockText:    acq.lockText,
			unlockText:  acq.unlockText,
			evoCall:     "evo." + call,
			pos:         acq.call.Pos(),
			guardsOther: anyDoesOtherWork(body, evoPkg),
		})
	}
	return regions
}

// syncMutexNames returns every field or variable name declared with type
// sync.Mutex/RWMutex (or a pointer to one), through the file's own sync
// import name.
func syncMutexNames(file *ast.File) map[string]bool {
	syncPkg := importNameFor(file, importSync)
	names := map[string]bool{}
	if syncPkg == "" {
		return names
	}
	record := func(t ast.Expr, idents []*ast.Ident) {
		if !isSyncMutexType(t, syncPkg) {
			return
		}
		for _, id := range idents {
			names[id.Name] = true
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			record(n.Type, n.Names)
		case *ast.ValueSpec:
			if n.Type != nil {
				record(n.Type, n.Names)
			}
		}
		return true
	})
	return names
}

func isSyncMutexType(t ast.Expr, syncPkg string) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	sel, ok := t.(*ast.SelectorExpr)
	return ok && isEvoIdent(sel.X, syncPkg) && (sel.Sel.Name == "Mutex" || sel.Sel.Name == "RWMutex")
}

// importNameFor returns the local name importPath is imported under in file, or
// "" when file does not import it.
func importNameFor(file *ast.File, importPath string) string {
	for _, imp := range file.Imports {
		if imp.Path.Value != `"`+importPath+`"` {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return importPath[strings.LastIndex(importPath, "/")+1:]
	}
	return ""
}

// lastName is the final identifier of a dotted expression: "mu" for w.mu.
func lastName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func isEvoManagedCall(n ast.Node, evoPkg string) bool {
	call, ok := n.(*ast.CallExpr)
	return ok && evoManagedCallName(call, evoPkg) != ""
}

// evoManagedCallName is "File", "Files", "Patch" or "Effect" when call is
// that function on the file's evo import, else "".
func evoManagedCallName(call *ast.CallExpr, evoPkg string) string {
	name, ok := strings.CutPrefix(calledFuncDotted(call), evoPkg+".")
	if !ok {
		return ""
	}
	if slices.Contains(evoManagedCalls, name) {
		return name
	}
	return ""
}

func firstEvoManagedCall(stmts []ast.Stmt, evoPkg string) string {
	found := ""
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && found == "" {
				found = evoManagedCallName(call, evoPkg)
			}
			return found == ""
		})
	}
	return found
}

// anyDoesOtherWork reports whether the region does anything besides
// building and calling evo.File: another call, a send, a goroutine, or a
// write through a field, index, or pointer (shared state the lock may be
// guarding). Local declarations, error checks, and returns are not work.
func anyDoesOtherWork(stmts []ast.Stmt, evoPkg string) bool {
	other := false
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if other || isEvoManagedCall(n, evoPkg) {
				return false
			}
			switch n := n.(type) {
			case *ast.CallExpr, *ast.IncDecStmt, *ast.SendStmt, *ast.GoStmt:
				other = true
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if _, local := lhs.(*ast.Ident); !local {
						other = true
					}
				}
			}
			return !other
		})
	}
	return other
}
