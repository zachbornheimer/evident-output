package review

import (
	"go/ast"
	"go/types"
)

const (
	importSync    = "sync"
	importOS      = "os"
	importSyscall = "syscall"
	importUnix    = "golang.org/x/sys/unix"
	importGofrs   = "github.com/gofrs/flock"
)

const (
	flagExclusiveLock = "LOCK_EX"
	flagSharedLock    = "LOCK_SH"
	flagUnlock        = "LOCK_UN"
	flagCreateExcl    = "O_EXCL"
)

// acquisition is one statement that takes a caller-managed lock, plus how to
// recognise its release.
type acquisition struct {
	call       *ast.CallExpr
	holder     string // what is held: "w.mu", "fd", the lock-file path
	lockText   string
	unlockText string
	// needsRelease means no region exists unless a release is found: an
	// O_EXCL lock file that is never removed is not a lock region.
	needsRelease bool
	releases     func(*ast.CallExpr) bool
	// upkeep is bookkeeping that is neither the release nor real work, e.g.
	// closing the lock-file handle.
	upkeep func(*ast.CallExpr) bool
}

// lockScan holds the per-file facts the matchers need: which names are
// mutexes or gofrs locks, and the local names of the os, syscall and unix
// packages.
type lockScan struct {
	mutexes, gofrs map[string]bool
	osPkg          string
	syscallPkg     string
	unixPkg        string
}

func newLockScan(file *ast.File) lockScan {
	return lockScan{
		mutexes:    syncMutexNames(file),
		gofrs:      gofrsLockNames(file),
		osPkg:      importNameFor(file, importOS),
		syscallPkg: importNameFor(file, importSyscall),
		unixPkg:    importNameFor(file, importUnix),
	}
}

// acquisition reports the lock that stmt takes, if it is one of the known
// shapes: mutex or gofrs Lock, flock(2), or an O_EXCL lock file.
func (s lockScan) acquisition(stmt ast.Stmt) (acquisition, bool) {
	call, assigned := stmtCall(stmt)
	if call == nil {
		return acquisition{}, false
	}
	matchers := []func(*ast.CallExpr, string) (acquisition, bool){s.lockMethod, s.flockCall, s.lockFile}
	for _, match := range matchers {
		if acq, ok := match(call, assigned); ok {
			return acq, true
		}
	}
	return acquisition{}, false
}

// lockMethod matches x.Lock()/x.RLock() on a sync mutex and
// x.Lock/TryLock/RLock/TryRLock on a gofrs/flock value.
func (s lockScan) lockMethod(call *ast.CallExpr, _ string) (acquisition, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) != 0 {
		return acquisition{}, false
	}
	holderName := lastName(sel.X)
	method := sel.Sel.Name
	shared := method == "RLock" || method == "TryRLock"
	mutexMethod := method == "Lock" || method == "RLock"
	gofrsMethod := mutexMethod || method == "TryLock" || method == "TryRLock"
	isMutex := s.mutexes[holderName] && mutexMethod
	isGofrs := s.gofrs[holderName] && gofrsMethod
	if !isMutex && !isGofrs {
		return acquisition{}, false
	}
	holder := exprDottedName(sel.X)
	unlock := "Unlock"
	if shared && isMutex {
		unlock = "RUnlock"
	}
	return acquisition{
		call:       call,
		holder:     holder,
		lockText:   types.ExprString(call),
		unlockText: holder + "." + unlock + "()",
		releases:   func(c *ast.CallExpr) bool { return isMethodCall(c, holder, unlock) },
	}, true
}

// flockCall matches syscall.Flock/unix.Flock taking LOCK_EX or LOCK_SH.
func (s lockScan) flockCall(call *ast.CallExpr, _ string) (acquisition, bool) {
	pkg := s.flockPackage(call)
	if pkg == "" || len(call.Args) != 2 || !mentionsAny(call.Args[1], flagExclusiveLock, flagSharedLock) {
		return acquisition{}, false
	}
	fd := call.Args[0]
	fdText := types.ExprString(fd)
	owner := fdOwner(fd)
	return acquisition{
		call:       call,
		holder:     fdText,
		lockText:   types.ExprString(call),
		unlockText: pkg + ".Flock(" + fdText + ", " + pkg + "." + flagUnlock + ")",
		releases: func(c *ast.CallExpr) bool {
			return isFlockUnlock(c, pkg, fdText) || isFdClose(c, pkg, fdText, owner)
		},
	}, true
}

// flockPackage is the local package name when call is a Flock call, else "".
func (s lockScan) flockPackage(call *ast.CallExpr) string {
	for _, pkg := range []string{s.syscallPkg, s.unixPkg} {
		if isPkgCall(call, pkg, "Flock") {
			return pkg
		}
	}
	return ""
}

func isFlockUnlock(c *ast.CallExpr, pkg, fdText string) bool {
	return isPkgCall(c, pkg, "Flock") && len(c.Args) == 2 &&
		types.ExprString(c.Args[0]) == fdText && mentionsAny(c.Args[1], flagUnlock)
}

// isFdClose reports a release by closing the descriptor: syscall.Close(fd),
// or f.Close() when fd is int(f.Fd()).
func isFdClose(c *ast.CallExpr, pkg, fdText, owner string) bool {
	if isPkgCall(c, pkg, "Close") && len(c.Args) == 1 && types.ExprString(c.Args[0]) == fdText {
		return true
	}
	return owner != "" && isMethodCall(c, owner, "Close")
}

// lockFile matches os.OpenFile(path, ...O_EXCL..., perm); the lock is held
// until os.Remove(path).
func (s lockScan) lockFile(call *ast.CallExpr, assigned string) (acquisition, bool) {
	if !isPkgCall(call, s.osPkg, "OpenFile") || len(call.Args) != 3 || !mentionsAny(call.Args[1], flagCreateExcl) {
		return acquisition{}, false
	}
	path := types.ExprString(call.Args[0])
	osPkg := s.osPkg
	return acquisition{
		call:         call,
		holder:       path,
		lockText:     types.ExprString(call),
		unlockText:   osPkg + ".Remove(" + path + ")",
		needsRelease: true,
		releases: func(c *ast.CallExpr) bool {
			return isPkgCall(c, osPkg, "Remove") && len(c.Args) == 1 && types.ExprString(c.Args[0]) == path
		},
		upkeep: func(c *ast.CallExpr) bool { return assigned != "" && isMethodCall(c, assigned, "Close") },
	}, true
}

// regionBody returns the statements the lock covers: those after the
// acquisition up to a bare release, or to the end of the block when the
// release is deferred. Release and upkeep statements are not part of it.
func regionBody(_ ast.Stmt, after []ast.Stmt, acq acquisition) (body []ast.Stmt, released bool) {
	for _, stmt := range after {
		if d, ok := stmt.(*ast.DeferStmt); ok {
			if acq.releases(d.Call) {
				released = true
				continue
			}
			if acq.upkeep != nil && acq.upkeep(d.Call) {
				continue
			}
			body = append(body, stmt)
			continue
		}
		if call, _ := stmtCall(stmt); call != nil {
			if acq.releases(call) {
				return body, true
			}
			if acq.upkeep != nil && acq.upkeep(call) {
				continue
			}
		}
		body = append(body, stmt)
	}
	return body, released
}

// stmtCall returns the call a statement makes and the first name it assigns:
// an expression statement, a single-call assignment, or an if initialiser.
func stmtCall(stmt ast.Stmt) (*ast.CallExpr, string) {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		call, _ := s.X.(*ast.CallExpr)
		return call, ""
	case *ast.AssignStmt:
		if len(s.Rhs) != 1 {
			return nil, ""
		}
		call, _ := s.Rhs[0].(*ast.CallExpr)
		name := ""
		if id, ok := s.Lhs[0].(*ast.Ident); ok && id.Name != "_" {
			name = id.Name
		}
		return call, name
	case *ast.IfStmt:
		if s.Init != nil {
			return stmtCall(s.Init)
		}
	}
	return nil, ""
}

// gofrsLockNames returns every name bound from flock.New/NewFlock through the
// file's gofrs import.
func gofrsLockNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	pkg := importNameFor(file, importGofrs)
	if pkg == "" {
		return names
	}
	isConstructor := func(e ast.Expr) bool {
		call, ok := e.(*ast.CallExpr)
		return ok && (isPkgCall(call, pkg, "New") || isPkgCall(call, pkg, "NewFlock"))
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range n.Rhs {
				if i < len(n.Lhs) && isConstructor(rhs) {
					names[lastName(n.Lhs[i])] = true
				}
			}
		case *ast.ValueSpec:
			for i, v := range n.Values {
				if i < len(n.Names) && isConstructor(v) {
					names[n.Names[i].Name] = true
				}
			}
		}
		return true
	})
	return names
}

func isMethodCall(call *ast.CallExpr, recv, method string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == method && exprDottedName(sel.X) == recv
}

// isPkgCall reports whether call is pkg.name(...) for a non-empty local
// package name.
func isPkgCall(call *ast.CallExpr, pkg, name string) bool {
	return pkg != "" && calledFuncDotted(call) == pkg+"."+name
}

// mentionsAny reports whether any identifier in n is one of idents.
func mentionsAny(n ast.Node, idents ...string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			for _, want := range idents {
				found = found || id.Name == want
			}
		}
		return !found
	})
	return found
}

// fdOwner is "f" when fd is int(f.Fd()) (or uintptr(f.Fd())), else "".
func fdOwner(fd ast.Expr) string {
	conv, ok := fd.(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 {
		return ""
	}
	inner, ok := conv.Args[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Fd" {
		return ""
	}
	return exprDottedName(sel.X)
}
