// Package review — Confirm prompts for destructive work that omit evo.Destructive (CONFIRM-002).
package review

import (
	"regexp"
	"strings"
)

// confirmCallPattern captures a Confirm call's question literal.
var confirmCallPattern = regexp.MustCompile(`Confirm\(\s*"([^"]*)"`)

// destructiveVerbPattern matches the severe-action verbs evo-rec.md's confirm
// gate calls out by name.
var destructiveVerbPattern = regexp.MustCompile(`(?i)delete|remove|trash|retire|force`)

// matchingParen returns the index of the ')' matching the '(' at openIdx, or
// -1 if unbalanced.
func matchingParen(src string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// detectConfirmMissingDestructive flags a Confirm question naming a severe
// action (delete/remove/trash/retire/force) without evo.Destructive() among
// its options — the "(destructive)" cue a user needs before approving it.
func detectConfirmMissingDestructive(filename, src string) []Finding {
	var findings []Finding
	for _, m := range confirmCallPattern.FindAllStringSubmatchIndex(src, -1) {
		question := src[m[2]:m[3]]
		if !destructiveVerbPattern.MatchString(question) {
			continue
		}
		openIdx := strings.Index(src[m[0]:], "(")
		if openIdx < 0 {
			continue
		}
		openIdx += m[0]
		closeIdx := matchingParen(src, openIdx)
		if closeIdx < 0 || strings.Contains(src[openIdx:closeIdx], "Destructive(") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "CONFIRM-002",
			Message:    "Confirm question reads as destructive but is missing evo.Destructive()",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "add evo.Destructive() to this Confirm call's options",
		})
	}
	return findings
}
