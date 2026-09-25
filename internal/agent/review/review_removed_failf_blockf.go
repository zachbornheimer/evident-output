// Package review — API-080 and API-081 (E-118 lane B): TaskHandle.Failf,
// TaskHandle.Blockf, Output.Failf, and the *Failure type they returned were
// removed in 1.1 with no compatibility alias. A consumer file pinned to an
// older release still calls them, so these fire only at or above
// dialectOneOne — the same gate API-032's own 1.1-only findings use.
package review

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// failfCausePattern matches the common shape this codebase's own call
// sites used for Failf (removed in 1.1): recv.Failf("summary: %w", cause)
// — the same %w-wrapped-cause shape evo.Cause(cause) carried, so the
// rewrite mirrors causeFindings'.
var failfCausePattern = regexp.MustCompile(`(\w+)\.Failf\(\s*"([^"]*):\s*%w"\s*,\s*([^()]*?)\s*\)`)

// blockfCausePattern is the same shape for Blockf (removed in 1.1), with
// an optional trailing .NextCommand(...)/.Next(...) chain — the remedy-
// attach methods the returned error value (use the returned error
// (errors.Is/As reach its %w cause directly) instead) used to carry,
// which no longer exist once Blockf is gone.
var blockfCausePattern = regexp.MustCompile(`(\w+)\.Blockf\(\s*"([^"]*):\s*%w"\s*,\s*([^()]*?)\s*\)(?:\.(NextCommand|Next)\(([^()]*(?:\([^()]*\))?[^()]*)\))?`)

// bareFailfBlockfPattern catches every other call to Failf(/Blockf( or
// Output.Fail (removed in 1.1, use Output.Fail) with a computed format
// string, so the removal is still flagged even when a derived rewrite is
// not cheap.
var bareFailfBlockfPattern = regexp.MustCompile(`\.(Failf|Blockf)\(`)

// detectRemovedFailfBlockf is API-080 (Failf, removed in 1.1) and API-081
// (Blockf, removed in 1.1), both removed with no compatibility alias
// (E-118 lane B). Fail and Block are statement-form now; there is no *f
// sibling in this family any more, and the value the returned error (use
// the returned error (errors.Is/As reach its %w cause directly) instead)
// used to carry (Next/NextCommand/Unwrap) is gone too.
func detectRemovedFailfBlockf(in fileInput) []Finding {
	if !dialectAtLeast(in.desiredVersion, dialectOneOne) {
		return nil
	}
	filename, src := in.filename, in.src
	var findings []Finding
	var derived []int

	for _, m := range failfCausePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]]
		if idx := strings.Index(src[m[0]:m[1]], ".Failf("); idx >= 0 {
			derived = append(derived, m[0]+idx)
		}
		var suggestion string
		if enclosingFuncReturnsError(in.file, in.fset, m[0]) {
			suggestion = fmt.Sprintf(`return fmt.Errorf(%q, %s)`, summary+": %w", cause)
		} else {
			suggestion = fmt.Sprintf(`%s.Fail(%q); return %s (this function has no error result to build with fmt.Errorf — keep the resolving Fail call and return the cause for its caller)`, recv, summary, cause)
		}
		findings = append(findings, Finding{
			RuleID:     "API-080",
			Message:    "TaskHandle.Failf was removed in 1.1 with no compatibility alias — Fail is statement-form; wrap the cause into a returned error instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}

	for _, m := range blockfCausePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]]
		if idx := strings.Index(src[m[0]:m[1]], ".Blockf("); idx >= 0 {
			derived = append(derived, m[0]+idx)
		}
		blockCall := fmt.Sprintf("%s.Block(%q)", recv, summary)
		if len(m) >= 12 && m[8] >= 0 {
			chainVerb, chainArgs := src[m[8]:m[9]], strings.TrimSpace(src[m[10]:m[11]])
			option := "evo.Next(" + chainArgs + ")"
			if chainVerb == "NextCommand" {
				option = "evo.NextCommand(" + chainArgs + ")"
			}
			blockCall = fmt.Sprintf("%s.Block(%q, %s)", recv, summary, option)
		}
		suggestion := fmt.Sprintf(`%s; return %s`, blockCall, cause)
		findings = append(findings, Finding{
			RuleID:     "API-081",
			Message:    "TaskHandle.Blockf was removed in 1.1 with no compatibility alias — Block is statement-form and stays the only way to conclude a Task Blocked; its remedy attaches as a ProblemOption on the Block call itself, not on a chained *Failure return",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}

	for _, m := range bareFailfBlockfPattern.FindAllStringIndex(src, -1) {
		if slices.Contains(derived, m[0]) {
			continue
		}
		verb := src[m[0]:m[1]]
		ruleID, message := "API-080", "TaskHandle.Failf/Output.Failf was removed in 1.1 with no compatibility alias — Fail is statement-form"
		if strings.Contains(verb, "Blockf") {
			ruleID, message = "API-081", "TaskHandle.Blockf was removed in 1.1 with no compatibility alias — Block is statement-form"
		}
		findings = append(findings, Finding{
			RuleID:     ruleID,
			Message:    message,
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace the statement-form verb (Fail/Block) plus a returned, %w-wrapped error from Define; attach a remedy as a Next/NextCommand ProblemOption on that same call, not on a chained return value",
		})
	}
	return findings
}
