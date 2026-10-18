// Package review — API-041 and API-044: caller goroutines and channel waits that rebuild what the scheduler already owns.
package review

import (
	"regexp"
	"strings"
)

// ===== API-041: a goroutine/.Go(func( closure reports on a Task nobody ever
// gave Define (Doing/Progress/Problem on it). The scheduler never received
// the work, so the Task never runs and the run ends partial (zq axis-11 P1).

// taskReportCallRe matches a status call on a named Task inside a closure.
var taskReportCallRe = regexp.MustCompile(`\b(\w+)\.(?:Doing|Progress|Problem)\(`)

// taskDefinedRe returns the pattern that proves name is given Define
// somewhere in src: a direct name.Define call, or a declaration whose chain
// already ends in Define.
func taskDefinedRe(name string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(name)
	return regexp.MustCompile(`\b` + quoted + `\.Define\(|\b` + quoted + `\s*:?=[^\n]*\.Define\(`)
}

func detectGoroutineResolvesPredeclaredTask(filename, src string) []Finding {
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
			if !strings.Contains(body, ".Define(") && reportsOnUndefinedTask(body, src) {
				findings = append(findings, Finding{
					RuleID:     "API-041",
					Message:    "goroutine/fan-out closure reports on a Task that is never given Define; the scheduler never received this work, so the Task does not run",
					File:       filename,
					Line:       lineAt(src, start),
					Suggestion: "declare the work with Group.Task(...) (one named Task per item) and call task.Define(func(ctx context.Context) error { ... }) instead of a bare goroutine",
				})
			}
			i = start + len(body)
		}
	}
	return findings
}

// reportsOnUndefinedTask reports whether closure calls Doing, Progress or
// Problem on a Task that src never gives Define.
func reportsOnUndefinedTask(closure, src string) bool {
	for _, m := range taskReportCallRe.FindAllStringSubmatch(closure, -1) {
		if !taskDefinedRe(m[1]).MatchString(src) {
			return true
		}
	}
	return false
}

// ===== API-044: a function that declares work with Define and then blocks
// on a channel the callback feeds has turned the declaring stack into a
// waiter. Everything declared after the wait is invisible to the scheduler
// and the dry-run plan until the callback has finished, and under
// MaxConcurrency 1 a waiter that itself runs inside a callback deadlocks.
// Ordering belongs to After; a produced value belongs to Computed.Get.

// channelDeclRe matches the channel a function makes for its callback to feed.
var channelDeclRe = regexp.MustCompile(`(\w+)\s*:?=\s*make\(chan\b`)

func detectChannelWaitWrapperAroundDefine(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		if !strings.Contains(fn.body, ".Define(") {
			continue
		}
		for _, m := range channelDeclRe.FindAllStringSubmatchIndex(fn.body, -1) {
			ch := fn.body[m[2]:m[3]]
			if !strings.Contains(fn.body, "<-"+ch) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "API-044",
				Message:    "this function declares work with Define and then blocks on a channel the callback feeds, so nothing declared after it is visible to the scheduler until the callback has finished",
				File:       filename,
				Line:       lineAt(src, fn.offset+m[0]),
				Suggestion: "delete the " + ch + " channel and the receive: keep task.Define(fn), order later work with After(task), and read a produced value with evo.Compute and Computed.Get",
			})
			break
		}
	}
	return findings
}
