// Package review — a Block used as an error value.
package review

import "strings"

// detectBlockedAsError flags control-flow that converts an expected Block/BlockedBy
// presentation outcome into a Go application error (MCP-014 / DOM-011).
// Real evaluation failures that use Fail/return before Block are not flagged.
func detectBlockedAsError(filename, src string) []Finding {
	// Fast reject: no block resolution → nothing to detect.
	if !strings.Contains(src, ".Block(") && !strings.Contains(src, ".BlockedBy(") {
		return nil
	}
	// Application-error constructors used after a blocked resolution.
	errorReturns := []string{
		"return errors.New(",
		"return fmt.Errorf(",
		"return errors.Join(",
		"return fmt.Error",
	}
	// Split into rough statements by newline for local ordering.
	lines := strings.Split(src, "\n")
	blockLine := -1
	for i, line := range lines {
		if strings.Contains(line, ".Block(") || strings.Contains(line, ".BlockedBy(") {
			// Fail path is application error — skip if same line is Fail.
			if strings.Contains(line, ".Fail(") {
				continue
			}
			blockLine = i
			break
		}
	}
	if blockLine < 0 {
		return nil
	}
	// Scan subsequent lines in the same function-ish region (until next func or EOF).
	for i := blockLine + 1; i < len(lines) && i < blockLine+40; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "func ") {
			break
		}
		// Returning Finish/conclusion is correct presentation closeout — not a false positive.
		if strings.Contains(line, "Finish(") || strings.Contains(line, "Conclusion()") || strings.Contains(line, "ExitCode") {
			continue
		}
		for _, pat := range errorReturns {
			if strings.Contains(line, pat) {
				return []Finding{{
					RuleID:     "DOM-011",
					Message:    "expected blocked item returned as application error; Block/BlockedBy is a presentation outcome — return nil after Finish, use conclusion ExitCode for process status (MCP-014)",
					File:       filename,
					Line:       i + 1,
					Suggestion: "replace this return with `return out.Finish()` and read the process status from Conclusion().ExitCode",
				}}
			}
		}
		// `return err` after Block when err is not from Finish — common misuse.
		if line == "return err" || strings.HasPrefix(line, "return err //") || line == "return err;" {
			// Allow if earlier line assigned err from Finish only in the window.
			finishAssigned := false
			for j := blockLine; j < i; j++ {
				if strings.Contains(lines[j], "Finish()") && strings.Contains(lines[j], "err") {
					finishAssigned = true
					break
				}
			}
			if !finishAssigned {
				return []Finding{{
					RuleID:     "DOM-011",
					Message:    "return err after Block treats expected blocked item as application error; Finish then use ExitCode (MCP-014)",
					File:       filename,
					Line:       i + 1,
					Suggestion: "replace `return err` with `return out.Finish()` and read the process status from Conclusion().ExitCode",
				}}
			}
		}
	}
	return nil
}
