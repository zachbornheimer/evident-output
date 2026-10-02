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
func detectBlockedAsError(filename, src string) []Finding {
	// Fast reject: no block resolution → nothing to detect.
	if !strings.Contains(src, ".Block(") && !strings.Contains(src, ".BlockedBy(") {
		return nil
	}
	lines := strings.Split(src, "\n")
	blockLine := firstBlockLine(lines)
	if blockLine < 0 {
		return nil
	}
	line, kind := blockedReturnAfter(lines, blockLine)
	if kind == noBlockedReturn {
		return nil
	}
	// Block(summary, Detail/Next/NextCommand...) then `return err` keeps the
	// row blocked and Wait() returns that err: it records diagnostics, it does
	// not turn the outcome into a different error.
	if kind == returnsErr && blockCarriesOptions(lines[blockLine:line]) {
		return nil
	}
	return []Finding{blockedAsErrorFinding(filename, line+1, kind)}
}

var blockOptionMarkers = []string{"evo.Detail(", "evo.Next(", "evo.NextCommand("}

// blockCarriesOptions reports whether the Block statement in lines has a
// ProblemOption that adds information beyond its summary.
func blockCarriesOptions(lines []string) bool {
	text := strings.Join(lines, "\n")
	for _, m := range blockOptionMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

// firstBlockLine is the index of the first line resolving a Task Blocked,
// or -1. A line that also Fails is an application error, not a Block.
func firstBlockLine(lines []string) int {
	for i, line := range lines {
		if (strings.Contains(line, ".Block(") || strings.Contains(line, ".BlockedBy(")) && !strings.Contains(line, ".Fail(") {
			return i
		}
	}
	return -1
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
