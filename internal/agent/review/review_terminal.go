// Package review — signal wiring, terminal passthrough, and hand-rolled confirm prompts (SIG, TERM-015, CONFIRM-001).
package review

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// detectSignalNotifyWithoutCancel flags signal.Notify in an evo-using file
// that never calls Cancel — evo.Main/Output.Run already wires SIGINT/SIGTERM
// into Cancel on the active task so the ■ glyph and the process exit code
// (130) can never disagree; a hand-rolled signal.Notify that skips Cancel
// reopens that gap (evo-rec.md "Interrupts"). (evo.MainWith, which wired the same thing for an Isolated *Output, was removed in 1.0 in favor of Output.Run.)
func detectSignalNotifyWithoutCancel(filename, src string) []Finding {
	if !strings.Contains(src, "signal.Notify(") {
		return nil
	}
	if strings.Contains(src, ".Cancel(") {
		return nil
	}
	line := 1
	if before, _, ok := strings.Cut(src, "signal.Notify("); ok {
		line += strings.Count(before, "\n")
	}
	return []Finding{{
		RuleID:     "SIG-001",
		Message:    "signal.Notify without a Cancel call in this file; prefer evo.Main/Output.Run, which already wire SIGINT/SIGTERM into Cancel so the ledger and exit code agree",
		File:       filename,
		Line:       line,
		Suggestion: "replace the signal-handling goroutine with os.Exit(evo.Main(run)) or out.Run(ctx, run), or call task.Cancel(reason) from it",
	}}
}

// lifecycleSignalSelectors are the signal identifiers that overlap the
// SIGINT/SIGTERM cancellation evo.Main/evo.Run already wire into RunFunc's
// context — os.Interrupt, syscall.SIGINT, syscall.SIGTERM. Any other signal
// (SIGHUP, SIGUSR1, ...) is unrelated application signal handling and
// detectDuplicateSignalWiringAroundMain never flags it.
var lifecycleSignalSelectors = map[string]bool{
	"Interrupt": true,
	"SIGINT":    true,
	"SIGTERM":   true,
}

// detectDuplicateSignalWiringAroundMain flags signal.Notify/NotifyContext
// calls that wire SIGINT/SIGTERM/os.Interrupt in a file that also calls
// evo.Main/evo.Run — a host-built interrupt layer solely duplicating the
// lifecycle those entrypoints already own (evo-rec.md "Interrupts";
// Decisions 2026-09-23, ZYS-939: "evo.Main / evo.Run already own SIGINT/
// SIGTERM cancellation and second-signal behavior. Flag host code that
// wraps the callback in its own signal.NotifyContext / duplicate interrupt
// layer solely for Evo lifecycle."). Signal handling for anything else
// (SIGHUP, SIGUSR1, ...) is real application behavior and is left alone.
func detectDuplicateSignalWiringAroundMain(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" || !fileCallsEvoEntrypoint(file, pkg, "Main", "Run") {
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
// names — e.g. evo.Main(...) or evo.Run(...).
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
		Message:    "signal." + verb + " wires SIGINT/SIGTERM/os.Interrupt in a file that also calls " + pkg + ".Main/" + pkg + ".Run; those entrypoints already cancel RunFunc's context on the same signals, so this duplicate layer can let the ledger and the process's actual exit path diverge",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "delete the signal." + verb + " call and read cancellation from the ctx " + pkg + ".Main/" + pkg + ".Run already passes into the run callback; keep signal.Notify only for signals unrelated to Evo's own lifecycle (e.g. SIGHUP)",
	}
}

// detectTTYPassthroughWithoutSuspend flags exec.Cmd Stdout/Stderr wired
// directly to the process's inherited terminal (tty passthrough) in a file
// that holds an active evo Output but never calls Suspend — two processes
// painting the same terminal glue the child's first line to the parent's
// live spinner (evo-rec.md "#7b"). Captured children (Writer/Capture)
// are unaffected and never match this pattern.
func detectTTYPassthroughWithoutSuspend(filename, src string) []Finding {
	if !strings.Contains(src, "exec.Command(") {
		return nil
	}
	hasPassthrough := strings.Contains(src, ".Stdout = os.Stdout") || strings.Contains(src, ".Stderr = os.Stderr")
	if !hasPassthrough {
		return nil
	}
	if strings.Contains(src, ".Writer()") || strings.Contains(src, "task.Run(") {
		return nil
	}
	line := 1
	idx := strings.Index(src, ".Stdout = os.Stdout")
	if idx < 0 {
		idx = strings.Index(src, ".Stderr = os.Stderr")
	}
	if idx >= 0 {
		line += strings.Count(src[:idx], "\n")
	}
	return []Finding{{
		RuleID:     "TERM-015",
		Message:    "tty-passthrough child (Stdout/Stderr inherited); capture it with task.Writer() so the live row keeps moving",
		File:       filename,
		Line:       line,
		Suggestion: "cmd.Stdout = task.Writer(); cmd.Stderr = task.Writer()",
	}}
}

// detectHandRolledConfirm flags a bespoke stdin prompt (bufio.NewReader(os.Stdin)
// or fmt.Scan*) in a file that imports evo — the same gate evo.Confirm already
// owns (spinner pause, prompt line, OK/declined/blocked resolution).
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
		Message:    "hand-rolled stdin confirm prompt in a file that imports evo; use evo.Confirm for spinner-pause + OK/declined/blocked resolution",
		File:       filename,
		Line:       line,
		Suggestion: "replace the bufio/fmt.Scan prompt with evo.Confirm(question, evo.AssumeYes(flagYes))",
	}}
}
