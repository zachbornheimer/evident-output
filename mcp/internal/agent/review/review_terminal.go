// Package review — duplicate signal wiring and hand-rolled confirm prompts (SIG-002, CONFIRM-001).
package review

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// lifecycleSignalSelectors are the signal identifiers that overlap the
// SIGINT/SIGTERM cancellation evo.Main already wires into the run's context:
// os.Interrupt, syscall.SIGINT, syscall.SIGTERM. Any other signal (SIGHUP,
// SIGUSR1, ...) is unrelated application signal handling and
// detectDuplicateSignalWiringAroundMain never flags it.
var lifecycleSignalSelectors = map[string]bool{
	"Interrupt": true,
	"SIGINT":    true,
	"SIGTERM":   true,
}

// detectDuplicateSignalWiringAroundMain flags signal.Notify/NotifyContext
// calls that wire SIGINT/SIGTERM/os.Interrupt in a file that also calls
// evo.Main: a host-built interrupt layer duplicating the lifecycle Main
// already owns (it cancels the run's context on either signal and honours
// both until the run has finished). Signal handling for anything else
// (SIGHUP, SIGUSR1, ...) is real application behavior and is left alone.
func detectDuplicateSignalWiringAroundMain(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" || !fileCallsEvoEntrypoint(file, pkg, "Main") {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Notify" && sel.Sel.Name != "NotifyContext") || !isEvoIdent(sel.X, "signal") {
			return true
		}
		if !callArgsIncludeLifecycleSignal(call.Args) {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, duplicateSignalWiringFinding(filename, pos, pkg, sel.Sel.Name))
		return true
	})
	return findings
}

// fileCallsEvoEntrypoint reports whether file calls pkg.<name> for any of
// names — e.g. evo.Main(...) or evo.Execute(...).
func fileCallsEvoEntrypoint(file *ast.File, pkg string, names ...string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isEvoIdent(sel.X, pkg) {
			return true
		}
		if slices.Contains(names, sel.Sel.Name) {
			found = true
		}
		return true
	})
	return found
}

// callArgsIncludeLifecycleSignal reports whether any argument names a
// SIGINT/SIGTERM/os.Interrupt selector (lifecycleSignalSelectors).
func callArgsIncludeLifecycleSignal(args []ast.Expr) bool {
	for _, arg := range args {
		sel, ok := arg.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if lifecycleSignalSelectors[sel.Sel.Name] {
			return true
		}
	}
	return false
}

func duplicateSignalWiringFinding(filename string, pos token.Position, pkg, verb string) Finding {
	return Finding{
		RuleID:     "SIG-002",
		Message:    "signal." + verb + " wires SIGINT/SIGTERM/os.Interrupt in a file that also calls " + pkg + ".Main; Main already cancels the run's context on those signals, so this duplicate layer can let the ledger and the process's actual exit path diverge",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "delete the signal." + verb + " call and read cancellation from the ctx " + pkg + ".Main already passes to the run callback; keep signal.Notify only for signals unrelated to the run's lifecycle (e.g. SIGHUP)",
	}
}

// detectHandRolledConfirm flags a bespoke stdin prompt (bufio.NewReader(os.Stdin)
// or fmt.Scan*) in a file that imports evo — the same gate Run.Confirm already
// owns (live-region pause, prompt line, a decline resolving the run Refused).
func detectHandRolledConfirm(filename, src string) []Finding {
	idx := strings.Index(src, "bufio.NewReader(os.Stdin)")
	if idx < 0 {
		idx = strings.Index(src, "fmt.Scan")
	}
	if idx < 0 {
		return nil
	}
	line := 1 + strings.Count(src[:idx], "\n")
	return []Finding{{
		RuleID:     "CONFIRM-001",
		Message:    "hand-rolled stdin confirm prompt in a file that imports evo; use r.Confirm, which pauses the live region, honors a non-interactive flag and resolves a decline as Refused",
		File:       filename,
		Line:       line,
		Suggestion: "replace the bufio/fmt.Scan prompt with r.Confirm(question, evo.NonInteractive(\"--yes\"))",
	}}
}
