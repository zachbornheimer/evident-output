// Package review — receiver, writer, and exit-code classifiers the selector-call and stream rules share.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

func strconvUnquote(s string) (string, error) {
	return strconv.Unquote(s)
}

// exprDottedName renders a simple dotted identifier chain (a.b.c) for an
// Ident or SelectorExpr receiver; returns "" for anything else (e.g. a call
// result), which intentionally excludes evo's own writer constructors
// (task.Capture(), out.Writer()) from the STREAM-003 indirection check —
// their return value is never bound to a stream-named identifier at the call
// site itself.
func exprDottedName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		base := exprDottedName(v.X)
		if base == "" {
			return v.Sel.Name
		}
		return base + "." + v.Sel.Name
	default:
		return ""
	}
}

// isOSStdStreamExpr reports whether expr is the literal os.Stdout/os.Stderr
// identifier — already the STREAM-003 exception for flag.Usage / pre-session
// errors.
func isOSStdStreamExpr(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr")
}

// evoOwnedWriterNames are evo's own writer-returning members; a Write call
// through one of these does not contaminate a managed stream because evo
// owns the destination (evo-rec.md "B": "except evo-owned writers").
var evoOwnedWriterNames = map[string]bool{"Writer": true, "ResultWriter": true}

// isEvoOwnedWriterExpr reports whether expr's final selector segment names
// one of evo's own writer accessors.
func isEvoOwnedWriterExpr(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return evoOwnedWriterNames[sel.Sel.Name]
}

// evoManagedWriterMethods are the 2.0 methods that return a writer Evo
// routes with the presentation: Run.Text, Run.ResultWriter, Task.Writer.
var evoManagedWriterMethods = map[string]bool{"Text": true, "ResultWriter": true, "Writer": true}

// isEvoManagedWriterCall reports whether expr is a call of an Evo
// managed-writer method (r.Text(), r.ResultWriter(), task.Writer()), the
// sanctioned target of fmt.Fprint*.
func isEvoManagedWriterCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && evoManagedWriterMethods[sel.Sel.Name]
}

// looksLikeStreamWriterName is the naming heuristic for STREAM-003's
// indirection widening (evo-rec.md "B"): a field/variable whose name reads
// like an output/error stream one hop from os.Stdout/os.Stderr (the real zq
// finding was a duplicate write to a field named services.Err). Scoped to
// stream-shaped names rather than "any io.Writer" so the detector stays
// honest instead of firing on ordinary bytes.Buffer/strings.Builder writers.
func looksLikeStreamWriterName(dotted string) bool {
	lower := strings.ToLower(dotted)
	last := lower
	if i := strings.LastIndex(lower, "."); i >= 0 {
		last = lower[i+1:]
	}
	if strings.Contains(lower, "testkit") {
		return false
	}
	switch last {
	case "err", "stderr", "errwriter", "out", "stdout", "outwriter":
		return true
	default:
		return false
	}
}

func isOSStderrArg(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "os" && sel.Sel.Name == "Stderr"
}

// localSafeWriterVars finds local variables declared or allocated as
// strings.Builder/bytes.Buffer — the STREAM-003 exception for fmt.Fprint*:
// writing into an in-memory buffer never touches a managed stream, so
// matching on the call name alone (ignoring the destination) is a false
// positive on ordinary buffered text-building code.
func localSafeWriterVars(f *ast.File) map[string]bool {
	vars := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.GenDecl:
			if decl.Tok != token.VAR {
				return true
			}
			for _, spec := range decl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || !isSafeWriterType(vs.Type) {
					continue
				}
				for _, name := range vs.Names {
					vars[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range decl.Rhs {
				if i >= len(decl.Lhs) {
					continue
				}
				ident, ok := decl.Lhs[i].(*ast.Ident)
				if ok && isSafeWriterAllocation(rhs) {
					vars[ident.Name] = true
				}
			}
		}
		return true
	})
	return vars
}

// isSafeWriterType reports whether a type expression is strings.Builder or
// bytes.Buffer in value form (as in "var sb strings.Builder").
func isSafeWriterType(t ast.Expr) bool {
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return (pkg.Name == "strings" && sel.Sel.Name == "Builder") ||
		(pkg.Name == "bytes" && sel.Sel.Name == "Buffer")
}

// isSafeWriterAllocation reports whether an assignment's right-hand side
// constructs a strings.Builder/bytes.Buffer, e.g. "&strings.Builder{}" or
// "bytes.Buffer{}".
func isSafeWriterAllocation(e ast.Expr) bool {
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.AND {
		e = u.X
	}
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return false
	}
	return isSafeWriterType(lit.Type)
}

// isSafeWriterArg reports whether a fmt.Fprint* destination argument is
// statically known to be an in-memory buffer — a local variable resolved by
// localSafeWriterVars, or an inline literal at the call site (e.g.
// "fmt.Fprintf(&bytes.Buffer{}, ...)").
func isSafeWriterArg(expr ast.Expr, safeVars map[string]bool) bool {
	if u, ok := expr.(*ast.UnaryExpr); ok && u.Op == token.AND {
		expr = u.X
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return safeVars[e.Name]
	case *ast.CompositeLit:
		return isSafeWriterType(e.Type)
	default:
		return false
	}
}

// knownNonEvoPackages are import idents that are never an evo receiver.
var knownNonEvoPackages = map[string]bool{
	"strings": true, "bytes": true, "regexp": true, "time": true,
	"context": true, "sync": true, "fmt": true, "os": true, "io": true,
	"path": true, "filepath": true, "unicode": true, "utf8": true,
	"sort": true, "slices": true, "maps": true, "http": true, "json": true,
	"errors": true, "log": true, "slog": true, "testing": true,
	"reflect": true, "runtime": true, "unsafe": true, "math": true,
	"strconv": true, "bufio": true, "compress": true, "crypto": true,
	"hash": true, "net": true, "url": true, "html": true, "flag": true,
	"exec": true, "signal": true, "atomic": true, "rand": true,
}

// isLikelyEvoReceiver is a soft receiver check: it accepts anything that
// looks like a presentation handle and skips known stdlib packages.
func isLikelyEvoReceiver(x ast.Expr) bool {
	switch v := x.(type) {
	case *ast.Ident:
		if knownNonEvoPackages[v.Name] {
			return false
		}
		switch v.Name {
		case "command", "cmd", "child", "process", "watcher", "harness", "grandchild", "proc", "pty":
			return false
		}
		return true
	case *ast.CallExpr:
		return true // out.Item("x").Start()
	case *ast.SelectorExpr:
		return isLikelyEvoReceiver(v.X)
	case *ast.ParenExpr:
		return isLikelyEvoReceiver(v.X)
	default:
		return true
	}
}

// isPresentationExitArg is true for os.Exit(evo.Main(...)) (Main derives the
// code but does not exit itself), os.Exit(...ExitCode()), os.Exit(x.ExitCode),
// and os.Exit(xe) where xe is a runExitCodeVars identifier. The last three
// also cover the exit-code-fidelity pattern (mcp/docs/exit-code-fidelity.md)
// that captures the run's code, branches to override it with a child
// process's own exit code, and only then calls os.Exit. All of these pass a
// code rather than invent one.
func isPresentationExitArg(call *ast.CallExpr, runCodeVars map[string]bool) bool {
	if len(call.Args) != 1 {
		return false
	}
	switch arg := call.Args[0].(type) {
	case *ast.CallExpr:
		return isRunExitCodeCall(arg)
	case *ast.SelectorExpr:
		// out.Conclusion().ExitCode or conc.ExitCode
		if arg.Sel.Name == "ExitCode" {
			return true
		}
	case *ast.Ident:
		return runCodeVars[arg.Name]
	}
	return false
}

// runExitCodeVars collects every identifier ever assigned the result of an
// evo.Main(...) or ....ExitCode() call: the variable the exit-code-fidelity
// pattern captures before optionally overriding it with a child process's
// own exit code. Tracking is file-wide, not scope-precise, matching
// localSafeWriterVars's tradeoff: a false pass here is a missed warning, not
// a wrong fix applied.
func runExitCodeVars(f *ast.File) map[string]bool {
	vars := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if i >= len(assign.Lhs) {
				continue
			}
			if ident, ok := assign.Lhs[i].(*ast.Ident); ok && isRunExitCodeCall(rhs) {
				vars[ident.Name] = true
			}
		}
		return true
	})
	return vars
}

// isRunExitCodeCall is true for evo.Main(...), which returns the derived
// code (os.Exit(evo.Main(cfg, run)) is the canonical entrypoint, not a
// wrapper flagged as double-exiting), and any ....ExitCode() call: the call
// shapes runExitCodeVars looks for on an assignment's right-hand side.
func isRunExitCodeCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		switch sel.Sel.Name {
		case "Main", "ExitCode":
			return true
		}
	}
	if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "Main" {
		return true
	}
	return false
}
