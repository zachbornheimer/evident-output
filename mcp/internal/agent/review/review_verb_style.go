// Package review — verb and summary style: names, placeholders, Fail/Block returns, Sprintf ceremony, wrappers, reasons, handles.
package review

import (
	"fmt"
	"regexp"
	"strings"
)

// taskDefineCallbackPattern matches a Task declared and given its Define
// callback in one expression, capturing the Task's name argument. The match
// ends at the callback's opening brace.
var taskDefineCallbackPattern = regexp.MustCompile(`\.Task\(([^(),]+)\)\.Define\(\s*func\([^)]*\)\s*error\s*\{`)

// endingReasonPattern matches the ending a callback returns, capturing the
// ending's verb and the expression it passes as its summary or reason.
var endingReasonPattern = regexp.MustCompile(`evo\.(Exclude|Fail|Refuse)\(\s*([^(),]+?)\s*[,)]`)

// detectNameEqualsVerbArgument is API-033: a Task whose Define callback
// returns an ending built from the identical expression that names the Task.
func detectNameEqualsVerbArgument(filename, src string) []Finding {
	var findings []Finding
	for _, m := range taskDefineCallbackPattern.FindAllStringSubmatchIndex(src, -1) {
		nameArg := strings.TrimSpace(src[m[2]:m[3]])
		body, _, ok := balancedBraceBody(src, m[1]-1)
		if !ok || nameArg == "" {
			continue
		}
		for _, e := range endingReasonPattern.FindAllStringSubmatch(body, -1) {
			if strings.TrimSpace(e[2]) != nameArg {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "API-033",
				Message:    "the same expression (" + nameArg + ") is used as both the Task name and the evo." + e[1] + "(...) argument — the second carries no new information",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: `give the Task a distinct name, e.g. .Task("<what this checks>"), and make the evo.` + e[1] + ` text say why`,
			})
			break
		}
	}
	return findings
}

// placeholderDoingPattern matches a bare-literal Doing call (no concatenation
// or Sprintf), the shape a placeholder narration string takes.
var placeholderDoingPattern = regexp.MustCompile(`\.Doing\(\s*"([^"]*)"\s*\)`)

// placeholderDoingWords are Doing strings that name no domain object — the
// user cannot tell this frame from the last one.
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
			Suggestion: `name the object in motion, e.g. Doing("scanning %s", name)`,
		})
	}
	return findings
}

// detectDiscardSinkInFailingBlock is API-035: io.Discard wired as a sink
// inside a function that also Fails/Refuses is an evidence-free security-gate
// shape — the verdict has nothing to show for itself.
func detectDiscardSinkInFailingBlock(filename, src string) []Finding {
	var findings []Finding
	for _, fb := range allFuncBodies(src) {
		if !strings.Contains(fb.body, "io.Discard") {
			continue
		}
		if !strings.Contains(fb.body, ".Refuse(") && !strings.Contains(fb.body, ".Fail(") {
			continue
		}
		idx := strings.Index(fb.body, "io.Discard")
		findings = append(findings, Finding{
			RuleID:     "API-035",
			Message:    "io.Discard sink in a function that also Fails/Refuses discards the evidence a security gate needs to explain its own verdict",
			File:       filename,
			Line:       lineAt(src, fb.offset+idx),
			Suggestion: "wire the checked command's output through task.Writer() (on cmd.Stdout/Stderr) or evo.Exec instead of io.Discard, so the ending can carry the output as evo.Detail",
		})
	}
	return findings
}

// printfVariadicVerbPattern matches a call to a printf-variadic Task method
// (Doing) with fmt.Sprintf as (the start of) its argument list.
var printfVariadicVerbPattern = regexp.MustCompile(`(\w+)\.(Doing)\(\s*fmt\.Sprintf\(`)

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
// `return`-ing) exactly one call of a Task method: one of the narration
// verbs with an argument (group 1), or the argument-free Writer (group 2).
var wrapperMethodBodyPattern = regexp.MustCompile(`^(?:return\s+)?[\w.]+\.(?:(Doing|Progress|Fact|Summary|Problem)\([^{}]+\)|(Writer)\(\))\s*;?$`)

// detectWrapperMethod is API-037: a method whose entire body is one call on
// a Task adds a name and a stack frame over calling the verb directly (zq's
// resolutionPhase wrapper).
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
		verb := call[1] + call[2]
		if composesItsArgument(stmt) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-037",
			Message:    "method " + name + " wraps a single call (." + verb + "(...)) on a Task with no added behavior",
			File:       filename,
			Line:       lineAt(src, start),
			Suggestion: "inline ." + verb + "(...) at each caller and delete " + name,
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

// errTwicePattern matches `evo.Fail(errVar.Error(), evo.Detail(errVar.Error()))`
// (or Refuse, or Problem, with any options between) — the same error
// surfacing twice, once as the summary and once as the Detail.
var errTwicePattern = regexp.MustCompile(`(\w+)\.(Fail|Refuse|Problem)\(\s*(\w+)\.Error\(\)\s*,\s*(?:evo\.\w+\([^()]*\)\s*,\s*)*evo\.Detail\(\s*(\w+)\.Error\(\)\s*\)`)

// detectErrTwice is DOM-018: err.Error() as the summary alongside
// evo.Detail(err.Error()) shows the same error twice.
func detectErrTwice(filename, src string) []Finding {
	var findings []Finding
	for _, m := range errTwicePattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb := src[m[2]:m[3]], src[m[4]:m[5]]
		errVar, detailVar := src[m[6]:m[7]], src[m[8]:m[9]]
		if errVar != detailVar {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "DOM-018",
			Message:    errVar + ".Error() as the summary and again in evo.Detail surfaces the same error twice",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "give the summary a short context and keep the error text only in the Detail: " + recv + "." + verb + "(\"<what failed>\", evo.Detail(" + errVar + ".Error()))",
		})
	}
	return findings
}

// dynamicReasonCallPattern matches an evo.Exclude( call.
var dynamicReasonCallPattern = regexp.MustCompile(`evo\.Exclude\(`)

// simpleReasonArgPattern matches the two shapes that keep evo.Exclude's
// cardinality bounded: a string literal, or a bare (optionally
// package-qualified) identifier — a const or package-level var.
var simpleReasonArgPattern = regexp.MustCompile(`^(?:"[^"]*"|[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)?)$`)

// detectDynamicReason is TAX-002: evo.Exclude built from a computed
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
			Message: "evo.Exclude reason built from a computed expression (" + firstArg +
				") is a cardinality bug — each distinct rendered value opens a new taxonomy bucket",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `use a fixed string literal naming the classification, e.g. evo.Exclude("protected"); fold the per-item detail into the item Task's name instead`,
		})
	}
	return findings
}

// entityNameLiteralPattern matches a Task, Group or Sequence declaration's
// literal name argument.
var entityNameLiteralPattern = regexp.MustCompile(`\.(?:Task|Group|Sequence)\(\s*"([^"]*)"`)

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

// crammedSummaryPattern matches the string literal summary of an evo.Fail,
// evo.Refuse, evo.Exclude or Task.Problem call.
var crammedSummaryPattern = regexp.MustCompile(`\.(Fail|Refuse|Exclude|Problem)\(\s*"([^"]*)"`)

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
			Suggestion: "split the text: keep the summary short, move the cause to evo.Detail(...) and the remedy to evo.Next(evo.Action{Label: ...})",
		})
	}
	return findings
}
