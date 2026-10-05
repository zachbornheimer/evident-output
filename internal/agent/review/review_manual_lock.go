// Package review — API-055 (ZYS-931): a caller-managed sync.Mutex/RWMutex
// critical section that contains an evo.File call. File claims its own path
// for writing automatically (ZYS-840), so a lock that guards only the File
// call is redundant. A lock that also guards other shared state is real and
// must stay; holding it across File is still worth a warning, because File
// can wait on a resource claim while the caller's lock is held, a wait Evo
// cannot see or render.
//
// Detection walks the AST: the lock receiver must be declared as a
// sync.Mutex/RWMutex in the file, the region runs from Lock to its matching
// bare Unlock (or to the end of the block under a deferred Unlock), and the
// File call is matched through the file's own evo import name. Comments and
// strings can never match.
package review

import (
	"go/ast"
	"go/token"
)

// lockRegion is one Lock…Unlock critical section around an evo.File call.
type lockRegion struct {
	recv        string // dotted lock receiver, e.g. "w.mu"
	lock        string // "Lock()" or "RLock()"
	unlock      string // "Unlock()" or "RUnlock()"
	pos         token.Pos
	guardsOther bool // the region also does work besides the File call
}

func detectManualLockAroundEvoFile(filename string, file *ast.File, fset *token.FileSet) []Finding {
	evoPkg := evoImportName(file)
	mutexes := syncMutexNames(file)
	if evoPkg == "" || len(mutexes) == 0 {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for _, region := range fileLockRegions(block.List, mutexes, evoPkg) {
			findings = append(findings, manualLockFinding(filename, fset.Position(region.pos), region))
		}
		return true
	})
	return findings
}

// manualLockFinding words API-055 for region: removal only when the lock
// guards nothing but the File call, narrowing otherwise. It never suggests
// deleting the mutex field, which may guard other methods.
func manualLockFinding(filename string, pos token.Position, r lockRegion) Finding {
	pair := r.recv + "." + r.lock + "/" + r.recv + "." + r.unlock
	f := Finding{
		RuleID: "API-055",
		File:   filename,
		Line:   pos.Line,
		Column: pos.Column,
	}
	if r.guardsOther {
		f.Message = r.recv + " is held across an evo.File call; File may wait on its own resource claim while the caller's lock is held, a wait Evo cannot see"
		f.Suggestion = "keep " + r.recv + " for the shared state it guards, but end the critical section (" + r.recv + "." + r.unlock + ") before calling evo.File"
		return f
	}
	f.Message = r.recv + " locks around nothing but an evo.File call; File already claims its own path for writing with no caller code"
	f.Suggestion = "drop the " + pair + " pair around this evo.File call; File claims its path itself." +
		" For a non-File operation over the same path use evo.Effect(ctx, evo.EffectSpec{..., Resource: evo.FSResource(path)}, fn)"
	return f
}

// syncMutexNames returns every field or variable name declared with type
// sync.Mutex/RWMutex (or a pointer to one), through the file's own sync
// import name.
func syncMutexNames(file *ast.File) map[string]bool {
	syncPkg := importNameFor(file, "sync")
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

// importNameFor returns the local name path is imported under in file, or
// "" when file does not import it.
func importNameFor(file *ast.File, path string) string {
	for _, imp := range file.Imports {
		if imp.Path.Value != `"`+path+`"` {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return path
	}
	return ""
}

// fileLockRegions finds, among one block's statements, every Lock…Unlock
// region on a known mutex that contains an evo.File call.
func fileLockRegions(stmts []ast.Stmt, mutexes map[string]bool, evoPkg string) []lockRegion {
	var regions []lockRegion
	for i, stmt := range stmts {
		call, recv, method := mutexCall(stmt, mutexes)
		if method != "Lock" && method != "RLock" {
			continue
		}
		r := lockRegion{recv: recv, lock: method + "()", unlock: "Unlock()", pos: call.Pos()}
		if method == "RLock" {
			r.unlock = "RUnlock()"
		}
		body := lockedStatements(stmts[i+1:], recv, method)
		if !anyCallsEvoFile(body, evoPkg) {
			continue
		}
		r.guardsOther = anyDoesOtherWork(body, evoPkg)
		regions = append(regions, r)
	}
	return regions
}

// lockedStatements returns the statements after a Lock up to its matching
// bare Unlock, or to the end of the block when the Unlock is deferred. The
// deferred Unlock itself is not part of the region.
func lockedStatements(after []ast.Stmt, recv, lockMethod string) []ast.Stmt {
	unlock := "Unlock"
	if lockMethod == "RLock" {
		unlock = "RUnlock"
	}
	var body []ast.Stmt
	for _, stmt := range after {
		if d, ok := stmt.(*ast.DeferStmt); ok && isMutexMethodCall(d.Call, recv, unlock) {
			continue
		}
		if es, ok := stmt.(*ast.ExprStmt); ok {
			if call, ok := es.X.(*ast.CallExpr); ok && isMutexMethodCall(call, recv, unlock) {
				return body
			}
		}
		body = append(body, stmt)
	}
	return body
}

// mutexCall reports the call, dotted receiver, and method name when stmt
// is a bare method call on a known mutex, e.g. w.mu.Lock().
func mutexCall(stmt ast.Stmt, mutexes map[string]bool) (*ast.CallExpr, string, string) {
	es, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, "", ""
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil, "", ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !mutexes[lastName(sel.X)] {
		return nil, "", ""
	}
	return call, exprDottedName(sel.X), sel.Sel.Name
}

func isMutexMethodCall(call *ast.CallExpr, recv, method string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == method && exprDottedName(sel.X) == recv
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

func isEvoFileCall(n ast.Node, evoPkg string) bool {
	call, ok := n.(*ast.CallExpr)
	return ok && calledFuncDotted(call) == evoPkg+".File"
}

func anyCallsEvoFile(stmts []ast.Stmt, evoPkg string) bool {
	found := false
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if isEvoFileCall(n, evoPkg) {
				found = true
			}
			return !found
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
			if other || isEvoFileCall(n, evoPkg) {
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
