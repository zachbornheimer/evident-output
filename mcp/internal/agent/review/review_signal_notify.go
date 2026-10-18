// Package review — SIG-001: a signal handler that ends the process itself.
package review

import (
	"go/ast"
	"go/token"
)

// detectSignalNotifyWithoutCancel flags signal.Notify on a channel whose
// receiver calls os.Exit. Evo reconciles an interrupt through the run's
// context: the live frame shows the run Cancelled and the exit code is 130.
// A handler that exits the process directly skips both, so the report and
// the exit code can disagree.
func detectSignalNotifyWithoutCancel(filename string, file *ast.File, fset *token.FileSet) []Finding {
	if evoImportName(file) == "" {
		return nil
	}
	exiting := channelsReceivedBeforeExit(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isPackageCall(call, "signal", "Notify") || len(call.Args) == 0 {
			return true
		}
		if ch := identName(call.Args[0]); ch == "" || !exiting[ch] {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "SIG-001",
			Message:    "this signal handler ends the process with os.Exit; the interrupt never reaches the run, so the ledger and the exit code can disagree",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "delete the handler and use os.Exit(evo.Main(cfg, run)), which cancels the run's context on SIGINT and SIGTERM and exits 130; with evo.Execute, cancel the context you pass instead of exiting",
		})
		return true
	})
	return findings
}

// channelsReceivedBeforeExit names every channel that a function literal
// receives from or ranges over and that also calls os.Exit.
func channelsReceivedBeforeExit(file *ast.File) map[string]bool {
	chans := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok || !callsPackageFunc(lit.Body, "os", "Exit") {
			return true
		}
		ast.Inspect(lit.Body, func(m ast.Node) bool {
			switch m := m.(type) {
			case *ast.UnaryExpr:
				if m.Op == token.ARROW {
					chans[identName(m.X)] = true
				}
			case *ast.RangeStmt:
				chans[identName(m.X)] = true
			}
			return true
		})
		return true
	})
	delete(chans, "")
	return chans
}

// isPackageCall reports whether call is pkg.name(...), by the package's
// local identifier.
func isPackageCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name && identName(sel.X) == pkg
}

// callsPackageFunc reports whether body contains a call to pkg.name.
func callsPackageFunc(body ast.Node, pkg, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && isPackageCall(call, pkg, name) {
			found = true
		}
		return !found
	})
	return found
}
