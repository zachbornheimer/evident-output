// Package review — API-080 and API-081 (E-118 lane B): TaskHandle.Failf,
// TaskHandle.Blockf, Output.Failf, and the *Failure type they returned were
// removed in 1.1 with no compatibility alias. A consumer file pinned to an
// older release still calls them, so these fire only at or above
// dialectOneOne — the same gate API-032's own 1.1-only findings use.
package review

import (
	"fmt"
	"go/ast"
	"go/token"
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
// attach methods that the removed *Failure return value used to carry,
// which no longer exist once Blockf is gone.
var blockfCausePattern = regexp.MustCompile(`(\w+)\.Blockf\(\s*"([^"]*):\s*%w"\s*,\s*([^()]*?)\s*\)(?:\.(NextCommand|Next)\(([^()]*(?:\([^()]*\))?[^()]*)\))?`)

// bareFailfBlockfPattern catches every other call to Failf(/Blockf( (both
// removed in 1.1) — TaskHandle.Failf and Output.Failf alike — with a
// computed format string, so the removal is still flagged even when a
// derived rewrite is not cheap.
var bareFailfBlockfPattern = regexp.MustCompile(`\.(Failf|Blockf)\(`)

// detectRemovedFailfBlockf is API-080 (Failf, removed in 1.1) and API-081
// (Blockf, removed in 1.1), both removed with no compatibility alias
// (E-118 lane B). Fail and Block are statement-form now; there is no *f
// sibling in this family any more, and the remedy actions the removed
// *Failure return value used to carry (Next/NextCommand/Unwrap) attach as
// ProblemOptions on the Fail/Block call itself instead.
func detectRemovedFailfBlockf(in fileInput) []Finding {
	if !dialectAtLeast(in.desiredVersion, dialectOneOne) {
		return nil
	}
	var findings []Finding
	var derived []int
	findings = append(findings, failfCauseFindings(in, &derived)...)
	findings = append(findings, blockfCauseFindings(in, &derived)...)
	findings = append(findings, bareFailfBlockfFindings(in, derived)...)
	return findings
}

// failfCauseFindings is API-080's derived-rewrite shape: a %w-wrapped-cause
// Failf call, rewritten the same way causeFindings rewrites evo.Cause(cause)
// — a returned, wrapped error inside a Define callback, or a statement-form
// Fail plus a bare return of the cause outside one.
func failfCauseFindings(in fileInput, derived *[]int) []Finding {
	filename, src := in.filename, in.src
	var findings []Finding
	for _, m := range failfCausePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]]
		if idx := strings.Index(src[m[0]:m[1]], ".Failf("); idx >= 0 {
			*derived = append(*derived, m[0]+idx)
		}
		var suggestion string
		if insideDefineResolvedCallback(in.file, in.fset, m[0]) && enclosingFuncReturnsError(in.file, in.fset, m[0]) {
			suggestion = fmt.Sprintf(`return fmt.Errorf(%q, %s)`, summary+": %w", cause)
		} else {
			suggestion = fmt.Sprintf(`%s.Fail(%q); return %s`, recv, summary, cause)
		}
		findings = append(findings, Finding{
			RuleID:     "API-080",
			Message:    "TaskHandle.Failf was removed in 1.1 with no compatibility alias — Fail is statement-form; wrap the cause into a returned error instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// blockfCauseFindings is API-081's derived-rewrite shape: a %w-wrapped-cause
// Blockf call, with an optional chained .Next/.NextCommand remedy rewritten
// onto the replacement Block call as ProblemOptions (blockfRemedyOptions),
// and the same Define-callback check failfCauseFindings uses to decide
// whether the cause is still returned.
func blockfCauseFindings(in fileInput, derived *[]int) []Finding {
	filename, src := in.filename, in.src
	var findings []Finding
	for _, m := range blockfCausePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]]
		if idx := strings.Index(src[m[0]:m[1]], ".Blockf("); idx >= 0 {
			*derived = append(*derived, m[0]+idx)
		}
		blockCall := fmt.Sprintf("%s.Block(%q)", recv, summary)
		if len(m) >= 12 && m[8] >= 0 {
			chainVerb, chainArgs := src[m[8]:m[9]], strings.TrimSpace(src[m[10]:m[11]])
			blockCall = fmt.Sprintf("%s.Block(%q, %s)", recv, summary, blockfRemedyOptions(chainVerb, chainArgs))
		}
		// Mirror failfCauseFindings' Define check (API-081's own GoodCode):
		// inside a Define/mutation callback, Block resolves the task and
		// the returned cause only lets Define hand it up, so `return
		// <cause>` stays correct there. Outside Define there is no
		// Output/Finish return value in scope for the cause to reach —
		// `return <cause>` there is exactly DOM-011's expected-blocked-
		// treated-as-application-error shape, so the rewrite must drop
		// the cause and return nil instead.
		var suggestion string
		if insideDefineResolvedCallback(in.file, in.fset, m[0]) && enclosingFuncReturnsError(in.file, in.fset, m[0]) {
			suggestion = fmt.Sprintf(`%s; return %s`, blockCall, cause)
		} else {
			suggestion = fmt.Sprintf(`%s; return nil`, blockCall)
		}
		findings = append(findings, Finding{
			RuleID:     "API-081",
			Message:    "TaskHandle.Blockf was removed in 1.1 with no compatibility alias — Block is statement-form and stays the only way to conclude a Task Blocked; its remedy attaches as a ProblemOption on the Block call itself, not on a chained *Failure return",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// blockfRemedyOptions rewrites a chained .Next(...)/.NextCommand(...) call
// (the removed *Failure's remedy-attach methods) into the ProblemOption
// arguments the replacement Block call takes instead.
func blockfRemedyOptions(chainVerb, chainArgs string) string {
	if chainVerb == "NextCommand" {
		// evo.NextCommand(executable string, args ...string) already takes
		// a variadic tail, so the whole argument list passes through as
		// one option.
		return "evo.NextCommand(" + chainArgs + ")"
	}
	// evo.Next(action Action) takes a single Action — the *Failure.Next
	// method (removed in 1.1 along with the rest of *Failure) took
	// variadic actions, so a multi-argument chain (`.Next(a, b)`) becomes
	// one evo.Next(...) option per action, not one evo.Next call holding
	// both arguments (which would not compile).
	actions := splitTopLevelArgs(chainArgs)
	opts := make([]string, len(actions))
	for i, a := range actions {
		opts[i] = "evo.Next(" + strings.TrimSpace(a) + ")"
	}
	return strings.Join(opts, ", ")
}

// bareFailfBlockfFindings is API-080/API-081's fallback shape: every other
// call to Failf(/Blockf( that failfCauseFindings/blockfCauseFindings did not
// already derive a rewrite for (derived), flagged with prose guidance
// instead of a computed suggestion.
func bareFailfBlockfFindings(in fileInput, derived []int) []Finding {
	filename, src := in.filename, in.src
	var findings []Finding
	for _, m := range bareFailfBlockfPattern.FindAllStringIndex(src, -1) {
		if slices.Contains(derived, m[0]) {
			continue
		}
		verb := src[m[0]:m[1]]
		insideDefine := insideDefineResolvedCallback(in.file, in.fset, m[0]) && enclosingFuncReturnsError(in.file, in.fset, m[0])
		var ruleID, message, suggestion string
		if strings.Contains(verb, "Blockf") {
			// Block always stays (it is the only way to conclude a Task
			// Blocked). Only the return alongside it differs by location
			// (API-081's GoodCode): inside Define, return the %w-wrapped
			// cause so Define resolves the task; outside Define, return
			// nil — a returned error there is DOM-011's expected-blocked-
			// treated-as-application-error shape, not Block's remedy.
			ruleID, message = "API-081", "TaskHandle.Blockf was removed in 1.1 with no compatibility alias — Block is statement-form and stays the only way to conclude a Task Blocked"
			if insideDefine {
				suggestion = "replace with Block(...) plus a returned, %w-wrapped error from Define (return err resolves the task via Define, DOM-011 exempts this shape); attach a remedy as a Next/NextCommand ProblemOption on the Block call itself, not on a chained return value"
			} else {
				suggestion = "replace with Block(...) plus return nil (there is no Output/Finish return value in scope outside Define, and a returned error here is DOM-011's expected-blocked-as-application-error shape); attach a remedy as a Next/NextCommand ProblemOption on the Block call itself, not on a chained return value"
			}
		} else {
			ruleID, message = "API-080", "TaskHandle.Failf/Output.Failf was removed in 1.1 with no compatibility alias — Fail is statement-form"
			suggestion = "replace with the statement-form verb (Fail) plus a returned, %w-wrapped error from Define; attach a remedy as a Next/NextCommand ProblemOption on that same call, not on a chained return value"
		}
		findings = append(findings, Finding{
			RuleID:     ruleID,
			Message:    message,
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// insideDefineResolvedCallback reports whether the byte offset off falls
// inside a Define callback's body — a FuncLit passed directly as Define's
// argument (evoResolutionCallbacks), or a same-file helper function
// reachable from one, per defineReachableBlocks. Only inside such a
// callback does a bare `return err` (or `return fmt.Errorf(...)`) actually
// resolve the task; a plain helper that merely happens to return error, but
// is never called from a Define callback, would leave the row unresolved
// if the Fail call were dropped, so API-080's rewrite must not drop it
// there.
func insideDefineResolvedCallback(file *ast.File, fset *token.FileSet, off int) bool {
	if file == nil || fset == nil {
		return false
	}
	tf := fset.File(file.Pos())
	if tf == nil || off < 0 || off > tf.Size() {
		return false
	}
	target := tf.Pos(off)

	for block := range defineReachableBlocks(file) {
		if block.Pos() <= target && target <= block.End() {
			return true
		}
	}
	return false
}
