// Package review — API-041, API-044, and API-052: caller goroutines, channel waits, and Wait loops that rebuild what the scheduler and container Wait already own.
package review

import "strings"

// ===== API-041: a goroutine/.Go(func( closure resolves a predeclared Task
// (Doing/Done/Fail/Progress) with no Define inside it — the scheduler never
// received the work (zq axis-11 P1).

var fanOutResolutionVerbMarkers = []string{".Doing(", ".Done(", ".Fail(", ".Progress("}

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
			if !strings.Contains(body, ".Define(") && containsAny(body, fanOutResolutionVerbMarkers) {
				findings = append(findings, Finding{
					RuleID:     "API-041",
					Message:    "goroutine/fan-out closure resolves a predeclared Task (Doing/Done/Fail/Progress) with no Define; evo never received this work to schedule",
					File:       filename,
					Line:       lineAt(src, start),
					Suggestion: "predeclare with Group.Task(...) (one named Task per item), then call task.Define(func() error { ... }) instead of a bare goroutine",
				})
			}
			i = start + len(body)
		}
	}
	return findings
}

// ===== API-044: a hand-rolled channel wrapper around Define reimplements
// TaskHandle.Wait() and hangs when the task is already terminal before
// Define runs (zq setup_python.go:190-210's defineAndWait; axis-3/15
// P15/P16 confirmed hang/deadlock). Wait is pre-approved here though it is
// landing in parallel and not yet in this module's public API — its
// spelling is fixed and it is the correct fix for this shape.

func detectChannelWaitWrapperAroundDefine(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		chanIdx := strings.Index(fn.body, "make(chan error")
		if chanIdx < 0 {
			continue
		}
		if !strings.Contains(fn.body, ".Define(") {
			continue
		}
		if !strings.Contains(fn.body, "<-") {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-044",
			Message:    "a channel-wait wrapper around Define reimplements task.Wait() and hangs when the task is already terminal before Define runs",
			File:       filename,
			Line:       lineAt(src, fn.offset+chanIdx),
			Suggestion: "replace the make(chan error)/Define/<-done wrapper with task.Define(fn); err := task.Wait()",
		})
	}
	return findings
}

// ===== API-052: a caller stores Group/Sequence child Task handles solely to
// loop Wait, filter ErrNotStarted, Snapshot the container, and hand-count
// failed children into its own aggregate error — GroupHandle.Wait/
// SequenceHandle.Wait (ZYS-849) now owns exactly this bookkeeping. zq
// evidence: internal/app/app.go::runParallel (slice of handles, Wait loop,
// Snapshot, failed-child count, "N of N failed" error) and
// internal/app/run_execute.go::waitDefinedRunOperations (Wait loop
// special-casing evo.ErrNotStarted, first-remaining-error return).

// callerWaitLoopSignals are the tokens that, alongside a for-loop calling
// .Wait() on a *TaskHandle, corroborate the container-boilerplate shape
// this rule targets rather than an unrelated Wait() loop (e.g. os/exec's
// Cmd.Wait()) — the rule requires the loop's function reference TaskHandle
// at all, plus at least one of these signals anywhere in the same function.
var callerWaitLoopSignals = []string{"ErrNotStarted", "Snapshot("}

func detectCallerWaitLoopOverContainerChildren(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		// The signature (parameter/receiver types, e.g. "tasks
		// []*evo.TaskHandle") sits before fn.body's opening brace, so the
		// TaskHandle/signal check reads the whole declaration, not only
		// the body statements.
		funcStart := strings.LastIndex(src[:fn.offset], "func ")
		if funcStart < 0 {
			funcStart = fn.offset
		}
		wholeFunc := src[funcStart : fn.offset+len(fn.body)]
		if !strings.Contains(wholeFunc, "TaskHandle") {
			continue
		}
		if !containsAny(wholeFunc, callerWaitLoopSignals) {
			continue
		}
		loop, waitIdx, ok := firstForLoopCallingWait(fn.body)
		if !ok {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-052",
			Message:    "a caller-owned loop waits on individually stored Task handles, filters ErrNotStarted, snapshots the container, and hand-counts failed children instead of using the container's own Wait",
			File:       filename,
			Line:       lineAt(src, fn.offset+loop.offset+waitIdx),
			Suggestion: "replace the stored-handle Wait loop and hand-counted aggregate error with the owning container's own GroupHandle.Wait()/SequenceHandle.Wait() (e.g. return jobs.Wait())",
		})
	}
	return findings
}

// firstForLoopCallingWait finds the first brace-balanced "for" loop in body
// whose block calls .Wait() directly, best-effort via textual scan (mirrors
// allFuncBodies' brace-balanced scan for "func"). It returns the loop's own
// funcBody, the byte offset of ".Wait()" within that loop body, and whether
// a match was found.
func firstForLoopCallingWait(body string) (loop funcBody, waitIdx int, ok bool) {
	scanFrom := 0
	for {
		rel := strings.Index(body[scanFrom:], "for ")
		if rel < 0 {
			return funcBody{}, 0, false
		}
		idx := scanFrom + rel
		if !precededByStatementBoundary(body, idx) {
			scanFrom = idx + len("for ")
			continue
		}
		block, start, balanced := balancedBraceBody(body, idx)
		if !balanced {
			scanFrom = idx + len("for ")
			continue
		}
		if waitIdx := strings.Index(block, ".Wait()"); waitIdx >= 0 {
			return funcBody{body: block, offset: start}, waitIdx, true
		}
		scanFrom = start + len(block)
	}
}

// precededByStatementBoundary reports whether the byte immediately before
// idx starts a new statement (newline, tab, space, or an opening brace) —
// filtering an identifier substring like "before " from matching "for ".
func precededByStatementBoundary(body string, idx int) bool {
	if idx == 0 {
		return true
	}
	switch body[idx-1] {
	case '\n', '\t', ' ', '{':
		return true
	default:
		return false
	}
}
