// Package review — API-082 (E-118 lane B): a chained TaskHandle.Next/
// NextCommand call right after Fail/Block on the same handle, where the
// ProblemOption form on the resolving call itself is canonical.
package review

import (
	"regexp"
	"strings"
)

// failBlockCallPattern matches a statement-form Fail/Block call and
// captures the receiver, so a following Next/NextCommand on the same
// receiver can be matched to it.
var failBlockCallPattern = regexp.MustCompile(`^(\w+)\.(Fail|Block)\(`)

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
	for i, line := range lines {
		m := failBlockCallPattern.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		recv, verb := m[1], m[2]
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
			findings = append(findings, Finding{
				RuleID: "API-082",
				Message: recv + "." + verb + "(...) followed by " + recv + "." + nm[2] +
					"(...) — fold the remedy into the " + verb + " call as an evo.Next/evo.NextCommand ProblemOption",
				File:       filename,
				Line:       j + 1,
				Suggestion: "move the " + nm[2] + "(...) arguments into " + recv + "." + verb + "(\"<summary>\", evo." + nm[2] + "(...)) and delete the chained call",
			})
			break
		}
	}
	return findings
}
