// Package review — API-054: hand-rolled subprocess capture around a Task instead of the ExecResult evo.Exec returns.
package review

import "strings"

// ===== API-054: generic bytes.Buffer/io.MultiWriter/task.Writer plumbing
// wired around a raw os/exec.Cmd solely to recreate Exec's own capture,
// liveness, and cancellation classification — or cancellation recognized by
// comparing captured output strings — when evo.Exec now returns an
// inspectable ExecResult (ZYS-850) that already owns exactly this. zq
// evidence: internal/app/run_captured_task.go allocates its own
// bytes.Buffer, combines task.Writer() with that buffer via io.MultiWriter,
// recognizes cancellation by comparing output strings, and classifies
// nonzero exit itself instead of inspecting ExecResult/ErrExecNonzeroExit.

// manualSubprocessCaptureCodeSignals are the code-shape tokens (identifiers/
// calls, never legitimate inside a string literal or comment) that, beside a
// function wiring a raw os/exec.Cmd's Stdout/Stderr to an Evo Task's own
// Writer(), corroborate the manual-recapture-of-Exec shape this rule
// targets: a hand-rolled buffer/multiwriter combine.
var manualSubprocessCaptureCodeSignals = []string{
	"bytes.Buffer", "bytes.NewBuffer", "MultiWriter(",
}

// manualSubprocessCaptureLiteralSignals are quoted signal text that, unlike
// manualSubprocessCaptureCodeSignals, is meant to be found as real string
// literal *content* in the code — the anti-pattern is comparing captured
// output against exactly this text (a cancellation check written as
// strings.Contains(out, "signal: killed") instead of an error/context
// check) — so, unlike the code-shape signals, these are matched against
// source with only comments masked, not string literals.
var manualSubprocessCaptureLiteralSignals = []string{
	`"signal: killed"`, `"signal: interrupt"`, `"context canceled"`,
}

// rawExecCmdSignals mark that the function drives a raw os/exec.Cmd (as
// opposed to some unrelated io.Writer plumbing) — required alongside
// task.Writer() so this rule only fires on code actually reimplementing
// Exec, not any bytes.Buffer/MultiWriter combination in the codebase. Like
// manualSubprocessCaptureCodeSignals, these are code-shape (an actual
// method call/type on a real exec.Cmd) and never legitimate as string
// literal or comment text, so they are matched with both masked out.
var rawExecCmdSignals = []string{"cmd.Run(", "cmd.Start(", "cmd.Output(", "cmd.CombinedOutput(", "exec.Cmd"}

func detectManualSubprocessCaptureAroundTask(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		funcStart := strings.LastIndex(src[:fn.offset], "func ")
		if funcStart < 0 {
			funcStart = fn.offset
		}
		wholeFunc := src[funcStart : fn.offset+len(fn.body)]
		// codeOnly blanks both comments and string literals so an
		// identifier-shaped signal (a real method call/type) only matches
		// when it is actually Go syntax, never text that merely mentions it
		// inside a log message, doc comment, or unrelated string literal.
		codeOnly := maskGoLexemes(wholeFunc, true)
		// literalsVisible blanks only comments, keeping string literal
		// content intact for signals that are meant to match real string
		// literal text in the source (the cancellation-string anti-pattern).
		literalsVisible := maskGoLexemes(wholeFunc, false)

		if !strings.Contains(codeOnly, ".Writer()") {
			continue
		}
		if !containsAny(codeOnly, rawExecCmdSignals) {
			continue
		}
		signal, idx := firstContainedToken(codeOnly, manualSubprocessCaptureCodeSignals)
		if signal == "" {
			signal, idx = firstContainedToken(literalsVisible, manualSubprocessCaptureLiteralSignals)
		}
		if signal == "" {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   "API-054",
			Severity: "error",
			Message:  "a raw os/exec.Cmd wired to an Evo Task's Writer() reimplements Exec's own capture/liveness/cancellation with hand-rolled " + signal + " plumbing instead of inspecting the ExecResult evo.Exec already returns",
			File:     filename,
			Line:     lineAt(src, funcStart+idx),
			Suggestion: "replace the raw exec.Cmd, its manual bytes.Buffer/io.MultiWriter capture, and any output-string cancellation match with " +
				"res, err := evo.Exec(ctx, spec); inspect res (ExecResult: Ran/ExitCode/Stdout/Stderr/Truncated) and errors.Is(err, evo.ErrExecNonzeroExit) instead",
		})
	}
	return findings
}
