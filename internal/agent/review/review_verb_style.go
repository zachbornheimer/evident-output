// Package review — verb and summary style: names, placeholders, Fail/Block returns, Sprintf ceremony, wrappers, reasons, handles.
package review

import (
	"fmt"
	"regexp"
	"strings"
)

// nameEqualsVerbArgPattern matches an entity declared and immediately
// resolved with the identical expression as both its name and its
// skip/verb argument, e.g. out.Item(note).Skip(note) — the owner's
// complaint that the second occurrence carries zero new information.
var nameEqualsVerbArgPattern = regexp.MustCompile(`\.(?:Item|Task)\(([^(),]+)\)\.(Skip|Fail|Block|Done|Cancel)\(([^(),]+)\)`)

// detectNameEqualsVerbArgument is API-033.
func detectNameEqualsVerbArgument(filename, src string) []Finding {
	var findings []Finding
	for _, m := range nameEqualsVerbArgPattern.FindAllStringSubmatchIndex(src, -1) {
		nameArg := strings.TrimSpace(src[m[2]:m[3]])
		verb := src[m[4]:m[5]]
		verbArg := strings.TrimSpace(src[m[6]:m[7]])
		if nameArg == "" || nameArg != verbArg {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-033",
			Message:    "the same expression (" + nameArg + ") is used as both the entity name and the ." + verb + "(...) argument — the second carries no new information",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `give the entity a distinct label, e.g. .Item("<what this checks>").` + verb + "(" + verbArg + ")",
		})
	}
	return findings
}

// placeholderDoingPattern matches a bare-literal Doing call (no concatenation
// or Sprintf), the shape a placeholder narration string takes.
var placeholderDoingPattern = regexp.MustCompile(`\.Doing\(\s*"([^"]*)"\s*\)`)

// placeholderDoingWords are Doing strings that name no domain object — the
// user cannot tell this frame from the last one (evo-rec.md "Doing carries
// the current object").
var placeholderDoingWords = map[string]bool{
	"starting": true, "working": true, "running": true, "please wait": true,
}

// detectPlaceholderDoing flags a Doing literal that is one of the generic
// placeholder words instead of naming the object currently in motion.
func detectPlaceholderDoing(filename, src string) []Finding {
	var findings []Finding
	for _, m := range placeholderDoingPattern.FindAllStringSubmatchIndex(src, -1) {
		lit := strings.ToLower(strings.TrimSpace(src[m[2]:m[3]]))
		if !placeholderDoingWords[lit] {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "FP-004",
			Message:    `Doing("` + lit + `") names no domain object; the user can't tell this frame from the last one`,
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `name the object in motion, e.g. Doing("scanning " + name)`,
		})
	}
	return findings
}

// failBlockStmtPattern matches a statement-form Fail/Block call (not
// Failf/Blockf, which already return the built error — the pattern requires
// "(" immediately after the verb name, which "Failf("/"Blockf(" never has).
var failBlockStmtPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(`)

// returnTheErrorSuggestion is how a Fail site hands its error back.
// Inside a Define or mutation callback the returned error is what resolves
// the task (API-040: resolving it first as well double-resolves); outside
// one, the f-form resolves the task and returns the error in one line.
func returnTheErrorSuggestion(recv, errVar string) string {
	return "inside a Define/mutation callback: `return fmt.Errorf(\"<context>: %w\", " + errVar + ")` and drop the " +
		recv + ".Fail call; elsewhere: `return " + recv + ".Failf(\"<context>: %w\", " + errVar + ")`"
}

// returnTheRefusalSuggestion is how a Block site hands its refusal back,
// inside a Define callback or not: Blockf resolves the Task Blocked and
// returns the refusal. A plain error would conclude it Failed instead, so a
// Block site is never rewritten to fmt.Errorf (E-105).
func returnTheRefusalSuggestion(blockf string) string {
	return "`" + blockf + "` in place of both lines: Blockf resolves the Task Blocked and returns the refusal, " +
		"inside a Define callback too (a plain error there would conclude the Task Failed)"
}

// handBackSuggestion is how a Fail or Block site returns errVar: the error
// for a Fail, the refusal caused by errVar for a Block.
func handBackSuggestion(recv, verb, errVar string) string {
	if verb == "Block" {
		return "`return " + recv + ".Blockf(\"<context>: %w\", " + errVar + ")`: Blockf resolves the Task Blocked and returns the refusal, " +
			"inside a Define callback too (a plain error there would conclude the Task Failed)"
	}
	return returnTheErrorSuggestion(recv, errVar)
}

// blockfCall rewrites a Block call's argument list as the Blockf call that
// returns the same refusal. A Block that carries ProblemOptions has no
// Blockf spelling, so ok is false.
func blockfCall(recv, args string) (call string, ok bool) {
	parts := splitTopLevelArgs(args)
	if len(parts) != 1 || strings.TrimSpace(parts[0]) == "" {
		return "", false
	}
	summary := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(summary, `"`) || strings.Contains(summary, "%") {
		summary = `"%s", ` + summary
	}
	return "return " + recv + ".Blockf(" + summary + ")", true
}

// detectFailBlockThenReturnNil is API-034: a statement-form Fail/Block
// followed immediately by a bare `return nil` discards the error the caller
// needed to propagate — the most common shape of "the remedy has nowhere to
// attach" (49 dotfiles + 41 zq sites).
func detectFailBlockThenReturnNil(filename, src string) []Finding {
	var findings []Finding
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		m := failBlockStmtPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		recv, verb := m[1], m[2]
		if sprintfInVerbPattern.MatchString(line) {
			continue // API-036 hands this one back in one line
		}
		suggestion := returnTheErrorSuggestion(recv, "err")
		if verb == "Block" {
			args, _, ok := balancedArgs(line, strings.Index(line, recv+".Block(")+len(recv+".Block"))
			blockf, ok2 := blockfCall(recv, args)
			if !ok || !ok2 {
				continue // a Block with ProblemOptions has no Blockf spelling
			}
			suggestion = returnTheRefusalSuggestion(blockf)
		}
		for j := i + 1; j < len(lines) && j < i+4; j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" || trimmed == "}" {
				continue
			}
			if trimmed == "return nil" {
				findings = append(findings, Finding{
					RuleID:     "API-034",
					Message:    recv + "." + verb + "(...) followed by return nil discards the error the caller needed to propagate",
					File:       filename,
					Line:       j + 1,
					Suggestion: suggestion,
				})
			}
			break
		}
	}
	return findings
}

// detectDiscardSinkInFailingBlock is API-035: io.Discard wired as a sink
// inside a function that also Fails/Blocks is an evidence-free security-gate
// shape — the verdict has nothing to show for itself.
func detectDiscardSinkInFailingBlock(filename, src string) []Finding {
	var findings []Finding
	for _, fb := range allFuncBodies(src) {
		if !strings.Contains(fb.body, "io.Discard") {
			continue
		}
		if !strings.Contains(fb.body, ".Block(") && !strings.Contains(fb.body, ".Fail(") {
			continue
		}
		idx := strings.Index(fb.body, "io.Discard")
		findings = append(findings, Finding{
			RuleID:     "API-035",
			Message:    "io.Discard sink in a function that also Fails/Blocks discards the evidence a security gate needs to explain its own verdict",
			File:       filename,
			Line:       lineAt(src, fb.offset+idx),
			Suggestion: "wire the checked command's output through task.Writer() (on cmd.Stdout/Stderr) or evo.Exec instead of io.Discard, so Block/Fail can attach DetailTail",
		})
	}
	return findings
}

// sprintfInVerbPattern matches Fail/Block called with fmt.Sprintf as (the
// start of) its argument list. Problem/Doing are deliberately excluded:
// they have no *f sibling and take fmt.Sprintf as an ordinary summary/text
// argument.
var sprintfInVerbPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*fmt\.Sprintf\(`)

// detectSprintfInVerb is API-036: a Fail/Block(fmt.Sprintf(...))
// statement followed by a return is the f-form's one job: resolve the task
// and return the formatted error in one line. Failf/Blockf return a
// *Failure, so the rewrite is offered only where that value is returned;
// a bare statement is already the right form (E-102: the rewrite's
// discarded *Failure failed errcheck), and ProblemOptions after the
// Sprintf have no f-form at all.
func detectSprintfInVerb(filename, src string) []Finding {
	var findings []Finding
	for _, m := range sprintfInVerbPattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb := src[m[2]:m[3]], src[m[4]:m[5]]
		openIdx := m[1] - 1
		args, endIdx, ok := balancedArgs(src, openIdx)
		if !ok {
			continue
		}
		rest := strings.TrimLeft(src[endIdx:], " \t")
		if !strings.HasPrefix(rest, ")") || !nextStatementReturns(rest[1:]) {
			continue
		}
		f := recv + "." + verb + "f(" + args + ")"
		suggestion := "outside a Define/mutation callback: `return " + f + "` in place of both lines; " +
			"inside one: `return fmt.Errorf(" + args + ")` and drop the " + recv + "." + verb + " call"
		if verb == "Block" {
			suggestion = returnTheRefusalSuggestion("return " + f)
		}
		findings = append(findings, Finding{
			RuleID:     "API-036",
			Message:    recv + "." + verb + "(fmt.Sprintf(...)) then return: " + recv + "." + verb + "f resolves the task and returns the error in one line",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// nextStatementReturns reports whether the statement after the one rest
// ends is a return.
func nextStatementReturns(rest string) bool {
	line, after, _ := strings.Cut(rest, "\n")
	if strings.TrimSpace(line) != "" {
		return false
	}
	next, _, _ := strings.Cut(strings.TrimLeft(after, " \t\n"), "\n")
	next = strings.TrimSpace(next)
	return next == "return" || strings.HasPrefix(next, "return ")
}

// printfVariadicVerbPattern matches a call to one of evo's own printf-
// variadic TaskHandle methods — Doing, Failf, and Blockf take
// (format string, args ...any) directly — with fmt.Sprintf as (the start
// of) its argument list.
var printfVariadicVerbPattern = regexp.MustCompile(`(\w+)\.(Doing|Failf|Blockf)\(\s*fmt\.Sprintf\(`)

// detectSprintfIntoVariadicVerb is API-038: fmt.Sprintf(...) passed to a
// method that is already printf-variadic itself is ceremony that also hides
// the real arguments from evo's own formatting — flatten it into the
// method's own format + args instead.
func detectSprintfIntoVariadicVerb(filename, src string) []Finding {
	var findings []Finding
	for _, m := range printfVariadicVerbPattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb := src[m[2]:m[3]], src[m[4]:m[5]]
		openIdx := strings.Index(src[m[0]:m[1]], "fmt.Sprintf(")
		if openIdx < 0 {
			continue
		}
		openIdx = m[0] + openIdx + len("fmt.Sprintf")
		args, endIdx, ok := balancedArgs(src, openIdx)
		if !ok {
			continue
		}
		rest := strings.TrimLeft(src[endIdx:], " \t\n")
		if !strings.HasPrefix(rest, ")") {
			// fmt.Sprintf isn't the sole argument (more options follow) — still
			// a real finding, but no cheap derived flattened call.
			findings = append(findings, Finding{
				RuleID:     "API-038",
				Message:    recv + "." + verb + "(fmt.Sprintf(...), ...) should flatten fmt.Sprintf into " + recv + "." + verb + "'s own format + args",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: recv + "." + verb + "(" + args + ")",
			})
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-038",
			Message:    recv + "." + verb + "(fmt.Sprintf(...)) should flatten into " + recv + "." + verb + "(...) directly",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: recv + "." + verb + "(" + args + ")",
		})
	}
	return findings
}

// methodDeclPattern matches a method declaration (has a receiver), capturing
// the method name so detectWrapperMethod can name it in the finding.
var methodDeclPattern = regexp.MustCompile(`func\s*\(\s*\w+\s+\*?\w+\s*\)\s+(\w+)\s*\(`)

// wrapperMethodBodyPattern matches a single statement that is (optionally
// `return`-ing) exactly one call ending in a known Task-verb method name.
var wrapperMethodBodyPattern = regexp.MustCompile(`^(?:return\s+)?[\w.]+\.(Doing|Done|Fail|Problem|Block|Cancel|Skip|Kept|Progress|Advance|Step|Evidence|Writer)\([^{}]*\)\s*;?$`)

// detectWrapperMethod is API-037: a method whose entire body is one call on
// a Task/Item handle adds a name and a stack frame over calling the verb
// directly (zq's resolutionPhase wrapper).
func detectWrapperMethod(filename, src string) []Finding {
	var findings []Finding
	for _, m := range methodDeclPattern.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		body, start, ok := balancedBraceBody(src, m[0])
		if !ok || len(body) < 2 {
			continue
		}
		inner := body[1 : len(body)-1]
		stmt := singleStatementBody(inner)
		if stmt == "" {
			continue
		}
		call := wrapperMethodBodyPattern.FindStringSubmatch(stmt)
		if call == nil {
			continue
		}
		if composesItsArgument(stmt) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-037",
			Message:    "method " + name + " wraps a single call (." + call[1] + "(...)) on a Task/Item handle with no added behavior",
			File:       filename,
			Line:       lineAt(src, start),
			Suggestion: "inline ." + call[1] + "(...) at each caller and delete " + name,
		})
	}
	return findings
}

// nestedCallArgumentPattern matches a call appearing inside the verb's own
// argument list — `fmt.Sprintf(...)`, `humanize(...)` — anywhere after the
// verb's opening parenthesis.
var nestedCallArgumentPattern = regexp.MustCompile(`\([^()]*[\w.]+\(`)

// composesItsArgument reports whether the wrapped verb's argument is built
// by the wrapper rather than passed straight through. Such a method is not a
// bare passthrough: a caller cannot inline the verb without copying the
// composition, so the name and the stack frame are earning their place.
func composesItsArgument(stmt string) bool {
	return nestedCallArgumentPattern.MatchString(stmt)
}

// singleStatementBody returns the sole non-blank, non-comment line of inner,
// or "" when inner has zero or more than one such line.
func singleStatementBody(inner string) string {
	var stmt string
	count := 0
	for l := range strings.SplitSeq(inner, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		stmt = t
		count++
	}
	if count != 1 {
		return ""
	}
	return stmt
}

// errTwicePattern matches `recv.Fail(errVar.Error(), evo.Cause(errVar))` (or
// Block) — the same error surfacing twice, once as bare summary text, once
// as the (now inert) Cause option.
var errTwicePattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*(\w+)\.Error\(\)\s*,\s*evo\.Cause\(\s*(\w+)\s*\)\s*\)`)

// detectErrTwice is DOM-018: err.Error() as the summary alongside
// evo.Cause(err) surfaces the same error twice — and since Fail/Block are
// statement-form, evo.Cause no longer affects the returned error at all, so
// the summary text and the (dead) cause option are now the identical string.
func detectErrTwice(filename, src string) []Finding {
	var findings []Finding
	for _, m := range errTwicePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb := src[m[2]:m[3]], src[m[4]:m[5]]
		errVar, causeVar := src[m[6]:m[7]], src[m[8]:m[9]]
		if errVar != causeVar {
			continue
		}
		findings = append(findings, Finding{
			RuleID: "DOM-018",
			Message: errVar + ".Error() as the summary and evo.Cause(" + errVar + ") as an option surface the same error twice; " +
				"evo.Cause no longer affects the returned error since Fail/Block are statement-form",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: handBackSuggestion(recv, verb, errVar),
		})
	}
	return findings
}

// dynamicReasonCallPattern matches a plain evo.Reason( call (not Reasonf,
// which is always format-built by design).
var dynamicReasonCallPattern = regexp.MustCompile(`evo\.Reason\(`)

// simpleReasonArgPattern matches the two shapes that keep evo.Reason's
// cardinality bounded: a string literal, or a bare (optionally
// package-qualified) identifier — a const or package-level var.
var simpleReasonArgPattern = regexp.MustCompile(`^(?:"[^"]*"|[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)?)$`)

// detectDynamicReason is TAX-002: evo.Reason built from a computed
// expression (concatenation, Sprintf, a Join over per-item data) opens one
// taxonomy bucket per distinct rendered value instead of one per
// classification — the live instance this closes: rr scan.go:60 joining
// per-item counts into the reason text.
func detectDynamicReason(filename, src string) []Finding {
	var findings []Finding
	for _, m := range dynamicReasonCallPattern.FindAllStringIndex(src, -1) {
		openIdx := m[1] - 1
		args, _, ok := balancedArgs(src, openIdx)
		if !ok {
			continue
		}
		parts := splitTopLevelArgs(args)
		if len(parts) == 0 {
			continue
		}
		firstArg := strings.TrimSpace(parts[0])
		if firstArg == "" || simpleReasonArgPattern.MatchString(firstArg) {
			continue
		}
		findings = append(findings, Finding{
			RuleID: "TAX-002",
			Message: "evo.Reason built from a computed expression (" + firstArg +
				") is a cardinality bug — each distinct rendered value opens a new taxonomy bucket",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `use a fixed string literal naming the classification, e.g. evo.Reason("protected"); fold the per-item detail into Skipped's name argument instead`,
		})
	}
	return findings
}

// entityNameLiteralPattern matches a Task/Item declaration's literal name
// argument.
var entityNameLiteralPattern = regexp.MustCompile(`\.(?:Task|Item)\(\s*"([^"]*)"`)

// longNameTransitionPattern flags an entity name narrating a transition
// (into/->) instead of naming a noun. "to" alone is deliberately excluded —
// it is common in ordinary noun phrases ("push to origin") and would
// false-positive on the overwhelming majority of legitimate names.
var longNameTransitionPattern = regexp.MustCompile(`(?i)\binto\b|->`)

// maxEntityNameLength is the guideline length beyond which a name reads as
// narration, not a noun (evo-rec.md "a Doing string narrates; a name labels").
const maxEntityNameLength = 40

// detectLongEntityName is TXT-020: an entity name over the length guideline,
// or narrating a transition, belongs in Doing/Donef — the name is a noun.
func detectLongEntityName(filename, src string) []Finding {
	var findings []Finding
	for _, m := range entityNameLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		var reason string
		switch {
		case len(name) > maxEntityNameLength:
			reason = fmt.Sprintf("is %d characters (over the ~%d character guideline)", len(name), maxEntityNameLength)
		case longNameTransitionPattern.MatchString(name):
			reason = "narrates a transition (into/->) instead of naming a noun"
		default:
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "TXT-020",
			Message:    fmt.Sprintf("entity name %q %s", name, reason),
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "shorten to a noun phrase; move the narrated detail into Doing(...) or the resolving verb's summary",
		})
	}
	return findings
}

// handleAssignPattern matches `name := recv.Task(...)` (or Item), capturing
// the variable name a live handle is bound to.
var handleAssignPattern = regexp.MustCompile(`\b(\w+)\s*:=\s*\w+\.(?:Task|Item)\(`)

// detectShadowedHandle is DOM-019: a Task/Item handle variable reassigned
// from a new declaration before the previous one was resolved orphans the
// earlier row Running forever — a double row hiding under one variable name.
func detectShadowedHandle(filename, src string) []Finding {
	var findings []Finding
	for _, fb := range allFuncBodies(src) {
		matches := handleAssignPattern.FindAllStringSubmatchIndex(fb.body, -1)
		lastDeclEnd := map[string]int{}
		for _, m := range matches {
			name := fb.body[m[2]:m[3]]
			if prevEnd, ok := lastDeclEnd[name]; ok {
				between := fb.body[prevEnd:m[0]]
				resolved := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\.(Done|Fail|Problem|Block|Cancel|Skip)\(`).MatchString(between)
				if !resolved {
					findings = append(findings, Finding{
						RuleID: "DOM-019",
						Message: "variable " + name + " is reassigned from a new Task/Item declaration before the previous one was resolved; " +
							"the earlier row is orphaned Running forever",
						File:       filename,
						Line:       lineAt(src, fb.offset+m[0]),
						Suggestion: "resolve " + name + " (Define, Fail, Block, Cancel, or Skipped) before reassigning it, or give the second declaration its own variable name",
					})
				}
			}
			lastDeclEnd[name] = m[1]
		}
	}
	return findings
}

// crammedSummaryPattern matches a Fail/Problem/Block string literal
// summary — Problem covers both severities (Warn was removed in 1.1; a
// warning is now Problem(summary, evo.Severity(evo.SeverityWarning)), and
// a crammed cause/action fragment is exactly as wrong at either severity).
var crammedSummaryPattern = regexp.MustCompile(`\.(Fail|Problem|Block)\(\s*"([^"]*)"`)

// detectCrammedSummary is TXT-021: a summary that hand-assembles a
// " — cause:"/" — action:" fragment reimplements Detail/Next inside plain text.
func detectCrammedSummary(filename, src string) []Finding {
	var findings []Finding
	for _, m := range crammedSummaryPattern.FindAllStringSubmatchIndex(src, -1) {
		verb := src[m[2]:m[3]]
		text := src[m[4]:m[5]]
		if !strings.Contains(text, "cause:") && !strings.Contains(text, "action:") {
			continue
		}
		if !strings.Contains(text, "—") && !strings.Contains(text, " - ") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "TXT-021",
			Message:    verb + " summary hand-assembles a cause/action fragment into the text instead of using Detail/Next",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "split the text: keep the summary short, move the cause to Detail(...) and the remedy to Next(evo.Label(...))",
		})
	}
	return findings
}
