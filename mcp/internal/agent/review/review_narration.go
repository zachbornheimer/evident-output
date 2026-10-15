// Package review — narration that restates what evo already shows: stale Doing text, hand-assembled counts and progress, fan-out declarations, hand-rolled writers.
package review

import (
	"regexp"
	"strings"
)

// subprocessRunMarkers are the calls that start an os/exec child a Task
// callback then waits on.
var subprocessRunMarkers = []string{"exec.Command(", "exec.CommandContext(", ".Run()"}

// detectStaleDoingBeforeSubprocess flags a function whose only Doing call
// sits ahead of an os/exec child with no further Doing/Progress/Writer — the
// spinner keeps animating over a silent child (FP-003).
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
		runIdx := earliestIndex(fn.body, subprocessRunMarkers)
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
// count string typically carries.
var taxonomyReasonPattern = regexp.MustCompile(`(?i)skipped|kept|retained`)

// sprintfLiteralPattern captures the format-string literal argument of an
// fmt.Sprintf call.
var sprintfLiteralPattern = regexp.MustCompile(`fmt\.Sprintf\(\s*"([^"]*)"`)

// detectHandAssembledTaxonomyCount flags fmt.Sprintf strings that bake a
// count into skip/keep/retain narration (e.g. "%d skipped") instead of
// declaring one Task per item that returns evo.Exclude(reason) and letting
// evo derive and sum the partition.
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
			Message:    "hand-assembled skip/keep count string; return evo.Exclude(reason) from one Task per item and let evo derive and sum the partition",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `replace with one Task per item, each returning evo.Exclude("..."); evo derives and sums the count`,
		})
	}
	return findings
}

// doingLiteralPattern captures a Doing call's string literal argument,
// whether passed directly or built via fmt.Sprintf.
var doingLiteralPattern = regexp.MustCompile(`\.Doing\(\s*(?:fmt\.Sprintf\()?\s*"([^"]*)"`)

// detectProgressInDoingString flags a Doing string smuggling "%d/%d" —
// progress hidden in narration text instead of a real Progress call
// (PROG-001).
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
// directly to Detail/Doing with the whole slice: Evo folds an Effect's names
// itself, but it cannot bound text the caller joined by hand.
func detectUnboundedSliceIntoNarration(filename, src string) []Finding {
	var findings []Finding
	for _, m := range sliceIntoNarrationPattern.FindAllStringSubmatchIndex(src, -1) {
		method := src[m[2]:m[3]]
		if joinsBoundedSlice(src, m[1]-1) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "BOUND-001",
			Message:    "strings.Join of an unbounded slice passed to " + method + "; bound the list before rendering",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "join only the first few names, e.g. names[:min(len(names), 8)], and report the rest as a count inside ." + method + "(...)",
		})
	}
	return findings
}

// joinsBoundedSlice reports whether the strings.Join call whose opening
// parenthesis sits at openIdx joins a slice expression (names[:n]), which
// is already bounded by the caller.
func joinsBoundedSlice(src string, openIdx int) bool {
	args, _, ok := balancedArgs(src, openIdx)
	if !ok {
		return false
	}
	return strings.Contains(splitTopLevelArgs(args)[0], "[:")
}

// fanOutClosureMarkers are the two shapes a fan-out worker closure takes:
// a bare goroutine, or a closure handed to an errgroup-style .Go(func...).
var fanOutClosureMarkers = []string{"go func(", ".Go(func("}

// detectTaskDeclaredInsideFanOut flags a Task declared inside a goroutine
// or g.Go closure. Declaration order is topology: it fixes the order of the
// rows and which siblings a later After can name. Declaring in a worker
// closure hands that order to goroutine scheduling, and inside a Define
// callback the declaration is refused outright.
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
					Message:    "Task declared inside a goroutine/fan-out closure; the order of the run's rows and After edges now follows goroutine scheduling",
					File:       filename,
					Line:       lineAt(src, start),
					Suggestion: "declare each Task in the enclosing function, in order, and give it Define there; Group already runs independent Tasks concurrently, so no goroutine is needed",
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
// method calls Task.Doing — reimplementing the exact line-splitting
// adapter Task.Writer already owns.
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
				Message:    "hand-rolled io.Writer.Write calls Task.Doing; use Task.Writer() instead",
				File:       filename,
				Line:       lineAt(src, start),
				Suggestion: "delete this Write method and wire the subprocess's Stdout/Stderr to task.Writer()",
			})
		}
	}
	return findings
}
