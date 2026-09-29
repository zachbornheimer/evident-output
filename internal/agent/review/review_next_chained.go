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
	lines := strings.Split(src, "\n")
	lineOffsets := computeLineOffsets(lines)
	var findings []Finding
	for i := range lines {
		recv, verb, callArgList, ok := failBlockCallOnLine(src, lines, lineOffsets, i)
		if !ok {
			continue
		}
		if finding, found := chainedNextFinding(filename, src, lines, lineOffsets, i, recv, verb, callArgList); found {
			findings = append(findings, finding)
		}
	}
	return findings
}

// computeLineOffsets returns each line's byte offset into the joined
// source, so a per-line match can be turned back into an absolute index for
// balancedArgs.
func computeLineOffsets(lines []string) []int {
	offsets := make([]int, len(lines))
	offset := 0
	for i, l := range lines {
		offsets[i] = offset
		offset += len(l) + 1
	}
	return offsets
}

// failBlockCallOnLine reports whether line i opens with a statement-form
// Fail/Block call, and reads that call's whole argument list via
// paren-balance rather than the trailing-`(` regex match alone — the
// summary argument may itself contain commas or parens.
func failBlockCallOnLine(src string, lines []string, lineOffsets []int, i int) (recv, verb string, callArgList []string, ok bool) {
	line := lines[i]
	trimmedLine := strings.TrimSpace(line)
	m := failBlockCallPattern.FindStringSubmatch(trimmedLine)
	if m == nil || !strings.HasPrefix(trimmedLine, m[1]+"."+m[2]+"(") {
		return "", "", nil, false
	}
	recv, verb = m[1], m[2]
	openIdx := lineOffsets[i] + strings.Index(line, recv+"."+verb+"(") + len(recv+"."+verb)
	callArgs, _, argsOK := balancedArgs(src, openIdx)
	if !argsOK {
		return "", "", nil, false
	}
	return recv, verb, splitTopLevelArgs(callArgs), true
}

// chainedNextFinding looks, within chainedNextScanWindow lines after i, for
// a bare Next/NextCommand statement on the same receiver as the Fail/Block
// call at i, and if found builds API-082's finding: the fold-in-as-
// ProblemOption rewrite, via nextChainProblemOptions (shared with
// blockfRemedyOptions's identical Next-vs-NextCommand shape).
func chainedNextFinding(filename, src string, lines []string, lineOffsets []int, i int, recv, verb string, callArgList []string) (Finding, bool) {
	summary := strings.TrimSpace(callArgList[0])
	for j := i + 1; j < len(lines) && j < i+chainedNextScanWindow; j++ {
		trimmed := strings.TrimSpace(lines[j])
		if trimmed == "" {
			continue
		}
		nm := chainedNextPattern.FindStringSubmatch(trimmed)
		if nm == nil || nm[1] != recv {
			return Finding{}, false
		}
		chainOpenIdx := lineOffsets[j] + strings.Index(lines[j], recv+"."+nm[2]+"(") + len(recv+"."+nm[2])
		chainArgs, _, chainOK := balancedArgs(src, chainOpenIdx)
		if !chainOK {
			return Finding{}, false
		}

		options := blockfRemedyOptions(nm[2], chainArgs)
		newArgs := append(append([]string{}, callArgList...), options)
		newCall := recv + "." + verb + "(" + strings.Join(newArgs, ", ") + ")"

		return Finding{
			RuleID: "API-082",
			Message: recv + "." + verb + "(...) followed by " + recv + "." + nm[2] +
				"(...) — fold the remedy into the " + verb + " call as an evo.Next/evo.NextCommand ProblemOption",
			File:       filename,
			Line:       j + 1,
			Suggestion: "replace both statements with " + newCall + " (summary: " + summary + ")",
		}, true
	}
	return Finding{}, false
}
