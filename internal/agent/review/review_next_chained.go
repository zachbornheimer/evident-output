// Package review — API-082 (E-118 lane B): a chained TaskHandle.Next/
// NextCommand call right after Fail/Block on the same handle, where the
// ProblemOption form on the resolving call itself is canonical.
package review

import (
	"regexp"
	"strings"
)

// failBlockCallPattern matches a statement-form Fail/Block call's opening
// and captures the receiver and verb; the call's own arguments are then
// read with balancedArgs so a multi-argument summary (or one containing
// commas/parens) is captured whole rather than truncated by this pattern.
var failBlockCallPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(`)

// chainedNextPattern matches a bare TaskHandle.Next/NextCommand statement
// (not the evo.Next/evo.NextCommand ProblemOption constructor, which is
// always an argument, never a statement on its own).
var chainedNextPattern = regexp.MustCompile(`^(\w+)\.(Next|NextCommand)\(`)

// chainedNextScanWindow is how many lines after Fail/Block the detector
// looks for a chained Next/NextCommand on the same receiver.
const chainedNextScanWindow = 3

// detectChainedNextAfterFailBlock is API-082: task.Fail(...)/task.Block(...)
// immediately followed by task.Next(...)/task.NextCommand(...) on the same
// handle is the one-statement case (remedy and resolving call together)
// spread across two calls — task.go documents the evo.Next/evo.NextCommand
// ProblemOption on the resolving call itself as canonical there.
func detectChainedNextAfterFailBlock(filename, src string) []Finding {
	var findings []Finding
	lines := strings.Split(src, "\n")
	lineOffsets := make([]int, len(lines))
	offset := 0
	for i, l := range lines {
		lineOffsets[i] = offset
		offset += len(l) + 1
	}
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		m := failBlockCallPattern.FindStringSubmatch(trimmedLine)
		if m == nil || !strings.HasPrefix(trimmedLine, m[1]+"."+m[2]+"(") {
			continue
		}
		recv, verb := m[1], m[2]

		// Read the Fail/Block call's own argument list, whole, via
		// paren-balance rather than the trailing-`(` regex match alone —
		// the summary argument may itself contain commas or parens.
		openIdx := lineOffsets[i] + strings.Index(line, recv+"."+verb+"(") + len(recv+"."+verb)
		callArgs, _, ok := balancedArgs(src, openIdx)
		if !ok {
			continue
		}
		callArgList := splitTopLevelArgs(callArgs)
		summary := strings.TrimSpace(callArgList[0])

		for j := i + 1; j < len(lines) && j < i+chainedNextScanWindow; j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" {
				continue
			}
			nm := chainedNextPattern.FindStringSubmatch(trimmed)
			if nm == nil {
				break
			}
			if nm[1] != recv {
				break
			}
			chainOpenIdx := lineOffsets[j] + strings.Index(lines[j], recv+"."+nm[2]+"(") + len(recv+"."+nm[2])
			chainArgs, _, chainOK := balancedArgs(src, chainOpenIdx)
			if !chainOK {
				break
			}

			var options string
			if nm[2] == "NextCommand" {
				// evo.NextCommand(executable string, args ...string)
				// already takes a variadic tail, so the whole argument
				// list passes through as one option.
				options = "evo.NextCommand(" + chainArgs + ")"
			} else {
				// evo.Next(action Action) takes exactly one Action —
				// TaskHandle.Next(a, b, ...) is variadic, so a
				// multi-argument chain becomes one evo.Next(...) option
				// per action (mirroring API-081's split of the same
				// variadic chain shape), not one evo.Next(...) holding
				// every argument, which would not compile.
				actions := splitTopLevelArgs(chainArgs)
				opts := make([]string, len(actions))
				for k, a := range actions {
					opts[k] = "evo.Next(" + strings.TrimSpace(a) + ")"
				}
				options = strings.Join(opts, ", ")
			}

			newArgs := append(append([]string{}, callArgList...), options)
			newCall := recv + "." + verb + "(" + strings.Join(newArgs, ", ") + ")"

			findings = append(findings, Finding{
				RuleID: "API-082",
				Message: recv + "." + verb + "(...) followed by " + recv + "." + nm[2] +
					"(...) — fold the remedy into the " + verb + " call as an evo.Next/evo.NextCommand ProblemOption",
				File:       filename,
				Line:       j + 1,
				Suggestion: "replace both statements with " + newCall + " (summary: " + summary + ")",
			})
			break
		}
	}
	return findings
}
