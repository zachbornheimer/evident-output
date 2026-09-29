// Package review — narration that restates what evo already shows: stale Doing text, hand-assembled counts and progress, fan-out declarations, hand-rolled writers.
package review

import (
	"regexp"
	"strings"
)

// detectStaleDoingBeforeSubprocess flags a task whose only Doing call sits
// ahead of a subprocess run with no further Doing/Progress/Writer — the
// spinner keeps animating over a silent child (evo-rec.md "FP-003").
func detectStaleDoingBeforeSubprocess(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		doingIdx := strings.Index(fn.body, ".Doing(")
		if doingIdx < 0 {
			continue
		}
		if strings.Count(fn.body, ".Doing(") != 1 {
			continue
		}
		if strings.Contains(fn.body, ".Writer(") {
			continue // child output wired to the live Doing; not stale
		}
		runIdx := earliestIndex(fn.body, []string{".Run(", "exec.Command("})
		if runIdx < 0 || runIdx < doingIdx {
			continue
		}
		after := fn.body[runIdx:]
		if strings.Contains(after, ".Doing(") || strings.Contains(after, ".Progress(") {
			continue
		}
		recv := identBefore(fn.body, doingIdx)
		suggestion := "wire the subprocess's Stdout/Stderr to Task.Writer(), or call Doing/Progress again after it exits"
		if recv != "" {
			suggestion = "wire the subprocess's Stdout/Stderr to " + recv + ".Writer(), or call " + recv + ".Doing(...)/" + recv + ".Progress(...) again after it exits"
		}
		findings = append(findings, Finding{
			RuleID:     "FP-003",
			Message:    "Doing is set once before a subprocess run with no further Doing/Progress/Writer; wire child output through Task.Writer or advance Doing as evidence arrives",
			File:       filename,
			Line:       lineAt(src, fn.offset+doingIdx),
			Suggestion: suggestion,
		})
	}
	return findings
}

// taxonomyReasonPattern matches the reason words a hand-assembled skip/keep
// count string typically carries (evo-rec.md "Taxonomy... derived, never
// assembled").
var taxonomyReasonPattern = regexp.MustCompile(`(?i)skipped|kept|retained`)

// sprintfLiteralPattern captures the format-string literal argument of an
// fmt.Sprintf call.
var sprintfLiteralPattern = regexp.MustCompile(`fmt\.Sprintf\(\s*"([^"]*)"`)

// detectHandAssembledTaxonomyCount flags fmt.Sprintf strings that bake a
// count into skip/keep/retain narration (e.g. "%d skipped") instead of
// recording reason + name via task.Skipped/Kept and letting evo derive and
// sum the partition.
func detectHandAssembledTaxonomyCount(filename, src string) []Finding {
	if !strings.Contains(src, "fmt.Sprintf(") {
		return nil
	}
	var findings []Finding
	for _, m := range sprintfLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
		lit := src[m[2]:m[3]]
		if !taxonomyReasonPattern.MatchString(lit) || !strings.Contains(lit, "%d") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "TAX-001",
			Message:    "hand-assembled skip/keep count string; record reason via task.Skipped and let evo derive and sum the partition",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `replace with task.Skipped(evo.Reason("...")) for each item; evo derives and sums the count`,
		})
	}
	return findings
}

// doingLiteralPattern captures a Doing call's string literal argument,
// whether passed directly or built via fmt.Sprintf.
var doingLiteralPattern = regexp.MustCompile(`\.Doing\(\s*(?:fmt\.Sprintf\()?\s*"([^"]*)"`)

// detectProgressInDoingString flags a Doing string smuggling "%d/%d" —
// progress hidden in narration text instead of a real Progress call
// (evo-rec.md "Additions" / PROG-001).
func detectProgressInDoingString(filename, src string) []Finding {
	var findings []Finding
	for _, m := range doingLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
		lit := src[m[2]:m[3]]
		if !strings.Contains(lit, "%d/%d") {
			continue
		}
		recv := identBefore(src, m[0])
		suggestion := "replace with Progress(completed, total)"
		if recv != "" {
			suggestion = "replace with " + recv + ".Progress(completed, total)"
		}
		findings = append(findings, Finding{
			RuleID:     "PROG-001",
			Message:    "Doing string smuggles a %d/%d count; use Progress(completed, total) so the count is structured, not narration text",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// sliceIntoNarrationPattern matches an unbounded strings.Join passed straight
// into Detail/Doing.
var sliceIntoNarrationPattern = regexp.MustCompile(`\.(Detail|Doing)\(\s*strings\.Join\(`)

// detectUnboundedSliceIntoNarration flags a strings.Join(slice, ...) passed
// directly to Detail/Doing without evo.TruncateNames — the same
// terminal flood evo-rec.md's bounded-rows fix already closed for
// Plan/Changes, one call site removed.
func detectUnboundedSliceIntoNarration(filename, src string) []Finding {
	if strings.Contains(src, "TruncateNames(") {
		return nil
	}
	var findings []Finding
	for _, m := range sliceIntoNarrationPattern.FindAllStringSubmatchIndex(src, -1) {
		method := src[m[2]:m[3]]
		findings = append(findings, Finding{
			RuleID:     "BOUND-001",
			Message:    "strings.Join of an unbounded slice passed to " + method + "; wrap it in evo.TruncateNames before rendering",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace strings.Join(...) with evo.TruncateNames(names, 8) inside ." + method + "(...)",
		})
	}
	return findings
}

// fanOutClosureMarkers are the two shapes a fan-out worker closure takes:
// a bare goroutine, or a closure handed to an errgroup-style .Go(func...).
var fanOutClosureMarkers = []string{"go func(", ".Go(func("}

// detectTaskDeclaredInsideFanOut flags out.Task/Group.Task called inside a
// goroutine or g.Go closure — declaring the Task there races task creation
// with rendering and produces the unordered multi-spinner defect evo-rec.md
// "predeclare children; present one Running" forbids.
func detectTaskDeclaredInsideFanOut(filename, src string) []Finding {
	var findings []Finding
	for _, marker := range fanOutClosureMarkers {
		for i := 0; i < len(src); {
			idx := strings.Index(src[i:], marker)
			if idx < 0 {
				break
			}
			idx += i
			body, start, ok := balancedBraceBody(src, idx)
			if !ok {
				break
			}
			if strings.Contains(body, ".Task(") {
				findings = append(findings, Finding{
					RuleID:     "API-030",
					Message:    "Task declared inside a goroutine/fan-out closure; predeclare all children before starting any goroutine",
					File:       filename,
					Line:       lineAt(src, start),
					Suggestion: "move the .Task(...) call above the goroutine/g.Go and pass the handle into the closure",
				})
			}
			i = start + len(body)
		}
	}
	return findings
}

// writeMethodDeclPattern matches a Write method declaration on any receiver
// type, the io.Writer interface shape a hand-rolled doing adapter implements.
var writeMethodDeclPattern = regexp.MustCompile(`func \([^)]*\)\s*Write\(`)

// detectHandRolledWriter flags a caller-defined io.Writer whose Write
// method calls TaskHandle.Doing — reimplementing the exact line-splitting
// adapter Task.Writer already owns (evo-rec.md "#6").
func detectHandRolledWriter(filename, src string) []Finding {
	var findings []Finding
	for _, loc := range writeMethodDeclPattern.FindAllStringIndex(src, -1) {
		body, start, ok := balancedBraceBody(src, loc[1])
		if !ok {
			continue
		}
		if strings.Contains(body, ".Doing(") {
			findings = append(findings, Finding{
				RuleID:     "API-031",
				Message:    "hand-rolled io.Writer.Write calls TaskHandle.Doing; use Task.Writer() instead",
				File:       filename,
				Line:       lineAt(src, start),
				Suggestion: "delete this Write method and wire the subprocess's Stdout/Stderr to task.Writer()",
			})
		}
	}
	return findings
}
