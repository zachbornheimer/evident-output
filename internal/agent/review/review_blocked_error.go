// Package review — a Block used as an error value.
package review

import "strings"

// blockedErrorReturns are application-error constructors that, returned
// after a blocked resolution, turn it into a Go error.
var blockedErrorReturns = []string{
	"return errors.New(",
	"return fmt.Errorf(",
	"return errors.Join(",
	"return fmt.Error",
}

// blockedScanWindow is how many lines after the Block the detector reads
// for a return, stopping early at the next func.
const blockedScanWindow = 40

// blockedReturnKind is how a return after a Block turned it into an error.
type blockedReturnKind uint8

const (
	noBlockedReturn blockedReturnKind = iota
	returnsNewError
	returnsErr
)

// detectBlockedAsError flags control-flow that converts an expected Block/BlockedBy
// presentation outcome into a Go application error (MCP-014 / DOM-011).
// Real evaluation failures that use Fail/return before Block are not flagged.
//
// It walks every Block/BlockedBy line in the file, not just the first: a
// file-wide early return on the first line's Define membership would
// suppress DOM-011 for every later run-level `Block; return err` in the
// same file whenever that first Block happened to sit inside Define.
func detectBlockedAsError(in fileInput) []Finding {
	// Fast reject: no block resolution → nothing to detect.
	if !strings.Contains(in.src, ".Block(") && !strings.Contains(in.src, ".BlockedBy(") {
		return nil
	}
	lines := strings.Split(in.src, "\n")
	var findings []Finding
	for i := range lines {
		if !isBlockLine(lines[i]) {
			continue
		}
		// Inside a Define callback, `task.Block(...); return err` is the
		// canonical, only-correct refusal shape (task.go's Block doc,
		// E-105): Block resolves the Task Blocked, and the returned error
		// is what lets Define propagate the failure without overriding
		// that resolution to Failed. There is no Output/Finish in scope to
		// redirect to, so DOM-011 does not apply there — only at the
		// Run/Main level where Finish exists.
		//
		// This reuses insideDefineResolvedCallback, the same AST-based
		// Define-membership owner API-080/API-081 use, instead of a
		// second, independent text brace-counter: a brace-counter can't
		// see braces inside strings/comments and can't follow a same-file
		// helper function reachable from a Define callback the way
		// defineReachableBlocks does.
		if insideDefineResolvedCallback(in.file, in.fset, lineOffset(lines, i)) {
			continue
		}
		line, kind := blockedReturnAfter(lines, i)
		if kind == noBlockedReturn {
			continue
		}
		findings = append(findings, blockedAsErrorFinding(in.filename, line+1, kind))
	}
	return findings
}

// lineOffset is the byte offset into the joined-by-"\n" src of the start of
// 0-indexed line i — the same reconstruction lineAt's line-splitting uses,
// inverted, so a line index found by scanning lines can be handed to the
// AST-based, byte-offset insideDefineResolvedCallback.
func lineOffset(lines []string, i int) int {
	off := 0
	for _, l := range lines[:i] {
		off += len(l) + 1 // +1 for the "\n" strings.Split dropped
	}
	return off
}

// isBlockLine reports whether line resolves a Task Blocked. A line that
// also Fails is an application error, not a Block.
func isBlockLine(line string) bool {
	return (strings.Contains(line, ".Block(") || strings.Contains(line, ".BlockedBy(")) && !strings.Contains(line, ".Fail(")
}

// blockedReturnAfter finds the first return after blockLine, within the
// same function, that hands the Block back as an error. Finish, Conclusion
// and ExitCode lines are the correct presentation closeout and never match.
func blockedReturnAfter(lines []string, blockLine int) (int, blockedReturnKind) {
	for i := blockLine + 1; i < len(lines) && i < blockLine+blockedScanWindow; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "func ") {
			break
		}
		if strings.Contains(line, "Finish(") || strings.Contains(line, "Conclusion()") || strings.Contains(line, "ExitCode") {
			continue
		}
		for _, pat := range blockedErrorReturns {
			if strings.Contains(line, pat) {
				return i, returnsNewError
			}
		}
		if isBareReturnErr(line) && !errFromFinish(lines[blockLine:i]) {
			return i, returnsErr
		}
	}
	return -1, noBlockedReturn
}

func isBareReturnErr(line string) bool {
	return line == "return err" || strings.HasPrefix(line, "return err //") || line == "return err;"
}

// errFromFinish reports whether err was assigned from Finish in lines.
func errFromFinish(lines []string) bool {
	for _, l := range lines {
		if strings.Contains(l, "Finish()") && strings.Contains(l, "err") {
			return true
		}
	}
	return false
}

// blockedAsErrorFinding is the DOM-011 finding for a return of kind at line.
func blockedAsErrorFinding(filename string, line int, kind blockedReturnKind) Finding {
	f := Finding{RuleID: "DOM-011", File: filename, Line: line}
	if kind == returnsErr {
		f.Message = "return err after Block treats expected blocked item as application error; Finish then use ExitCode (MCP-014)"
		f.Suggestion = "replace `return err` with `return out.Finish()` and read the process status from Conclusion().ExitCode"
		return f
	}
	f.Message = "expected blocked item returned as application error; Block/BlockedBy is a presentation outcome — return nil after Finish, use conclusion ExitCode for process status (MCP-014)"
	f.Suggestion = "replace this return with `return out.Finish()` and read the process status from Conclusion().ExitCode"
	return f
}
