package engine

import (
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// succeed resolves the task Done with summary: the engine's synchronous
// success verb for library-owned rows (Confirm's gate) and engine tests.
// The public TaskHandle has no equivalent since 1.1: callers resolve
// through Define, and Summary carries their result text.
func (t *TaskHandle) succeed(summary string) {
	t.finish(Done, txt.Text(summary), nil)
}

// Fail resolves the task as failed. This is a statement, not a fluent
// chain — Fail returns nothing, so a bare `task.Fail("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Use Failf
// to build and return a %w-wrapped error in one line.
func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	t.resolveWithProblem(Failed, summary, options)
}

// Failf resolves the task as failed with a formatted summary and returns a
// *Failure so a call site can `return` it directly:
// `return task.Failf("validate policy manifest: %w", err)`, and attach a
// remedy in the same statement: `.Next(evo.Label("..."))`. fmt.Errorf
// semantics: %w wraps its argument so errors.Is/As still reach it. See
// splitWrappedMessage for how a trailing ": %w"/", %w" splits the formatted
// text into the rendered summary and evidence line.
func (t *TaskHandle) Failf(format string, args ...any) *Failure {
	return t.resolveFormatted(Failed, format, args)
}

// resolveWithProblem is Fail and Block: resolve as state with one Problem
// built from summary and options.
func (t *TaskHandle) resolveWithProblem(state EntityState, summary string, options []ProblemOption) {
	p := applyProblemOptions(txt.Text(summary), options)
	t.finish(state, txt.Text(summary), []Problem{p})
}

// resolveFormatted is Failf and Blockf: resolve as state from a
// fmt.Errorf-formatted error and return it as a *Failure.
func (t *TaskHandle) resolveFormatted(state EntityState, format string, args []any) *Failure {
	err := fmt.Errorf(format, args...)
	summary, evidence := core.SplitWrappedMessage(format, err)
	problem := Problem{Summary: summary, Detail: evidence}
	t.attachRetainedEvidenceTail(&problem)
	t.finish(state, summary, []Problem{core.SanitizeProblem(problem)})
	return newFailure(t, err)
}

// attachRetainedEvidenceTail attaches the task's own retained evidence
// (Writer/PhaseWriter/capture()) as the Problem's
// EvidenceTail, the same precedence Capture.DetailTail() already
// documents: an existing Detail line — here, Failf/Blockf's own
// wrapped-error text — still renders as the primary line, and the retained
// evidence appends underneath rather than being silently dropped
// (beginner-gate-2 finding 3). Failf/Blockf accept no ProblemOptions, so
// this is the only way their call sites ever see the proof task.Writer()
// already captured; a bare Fail/Block with no Detail still gets its own auto-attach
// from finishTagged, unaffected by this.
func (t *TaskHandle) attachRetainedEvidenceTail(p *Problem) {
	if t == nil {
		return
	}
	t.capture().DetailTail().applyProblem(p)
}

// Block resolves the task as blocked. This is a statement, not a fluent
// chain — Block returns nothing, so a bare `task.Block("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Use
// Blockf to build and return a %w-wrapped error in one line.
func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	t.resolveWithProblem(Blocked, summary, options)
}

// Blockf resolves the task as blocked with a formatted summary and returns a
// *Failure exactly like Failf — see Failf for the fmt.Errorf %w,
// summary/evidence split, and Next/NextCommand remedy-attachment contract.
func (t *TaskHandle) Blockf(format string, args ...any) *Failure {
	return t.resolveFormatted(Blocked, format, args)
}

// Cancel resolves the task as cancelled. A nil *TaskHandle is safe and
// resolves nothing.
func (t *TaskHandle) Cancel(reason string) {
	t.finish(Cancelled, txt.Text(reason), nil)
}

// skip resolves the task as skipped (the engine body behind Skipped).
// reason is a printf format when args are present (fmt.Sprintf semantics).
func (t *TaskHandle) skip(reason string, args ...any) *TaskHandle {
	if len(args) > 0 {
		reason = fmt.Sprintf(reason, args...)
	}
	return t.finish(Skipped, txt.Text(reason), nil)
}

// failScheduled resolves the task Failed on the scheduler's authority,
// carrying the callback's error text as the row summary — Fail's shape,
// without Fail's caller-side submitted-task guard.
func (t *TaskHandle) failScheduled(summary string) {
	p := applyProblemOptions(txt.Text(summary), nil)
	t.resolveScheduled(Failed, txt.Text(summary), []Problem{p})
}

// doneScheduled resolves the task Done on the scheduler's authority — the
// one path that may declare a submitted task successful (see finish).
func (t *TaskHandle) doneScheduled() {
	t.resolveScheduled(Done, "", nil)
}

// finish is the caller-facing resolution path — every terminal verb
// (Fail/Block/Cancel/Skipped) a program writes by hand.
//
// Once a task is submitted, the scheduler owns the verdict on its work: only
// the callback's own return value says whether the work succeeded. So a
// hand-written Skipped on a submitted task is a *proposal*, not a
// resolution — it is held until the callback returns and then either
// ratified (the work did succeed; the caller's own summary is what renders,
// which is how a callback declares "✓ branches  8 deleted") or rejected as
// ErrAlreadyResolved misuse, with the observed failure taking the row (P2:
// an engine-side success resolution can no longer launder an error into a
// green row). Nothing ratifies its own completion.
//
// Bad news needs no ratification: Fail/Block/Cancel state an outcome the
// caller already knows and can only make the row worse, so they resolve
// immediately — that is how a callback reports its own failure (P13, where
// the scheduler must then not resolve it a second time) and how an interrupt
// cancels a running row.
func (t *TaskHandle) finish(state EntityState, summary string, problems []Problem) *TaskHandle {
	return t.resolve(state, summary, problems, byCaller)
}

// resolveScheduled is the scheduler's own resolution path, called only from
// executeWork once the callback has returned. It is the sole authority that
// may declare a submitted task successful.
func (t *TaskHandle) resolveScheduled(state EntityState, summary string, problems []Problem) {
	t.resolve(state, summary, problems, byScheduler)
}

// resolutionAuthority names who is resolving a task — the program that wrote
// the terminal verb, or the scheduler reporting what the callback returned.
type resolutionAuthority int

const (
	byCaller resolutionAuthority = iota
	byScheduler
)

// proposedOutcome is a caller's unratified success claim on a submitted
// task, held until the callback's return value confirms or contradicts it.
type proposedOutcome struct {
	state    EntityState
	summary  string
	problems []Problem
}

// declaresSuccess reports whether state claims the work went well — the
// class of claim only the scheduler's observation can ratify.
func declaresSuccess(state EntityState) bool {
	return state == Done || state == Skipped
}

// deniesItsOwnEffect reports whether this resolution is an evo.Effect
// callback disowning the work it was given: an Effect creating "module"
// whose fn calls Skipped or Fail and then returns nil rendered both `! skipped 1
// (install failed)` and `[changed] broken  created 1 module` — the ledger
// counting the package the installer had just rejected. A nil return after
// the row said "skipped" means "I handled it", not "I did it".
//
// Only the callback's own verdict counts. A later Fail from the program
// (an Effect in Define, then `task.Fail(...)`) and an interrupt that
// cancels a running Effect both describe work that really happened,
// and both still owe the reader `! already mutated: …`. The separator is
// the resolving goroutine's own stack: callbackDepth is non-zero only
// inside a task callback, which is precisely "the row resolved itself".
func deniesItsOwnEffect(st *taskState, state EntityState, authority resolutionAuthority) bool {
	if st.effectsInFlight == 0 || st.sched.phase != phaseRunning || authority != byCaller || state == Done {
		return false
	}
	return callbackDepth() > 0
}

func (t *TaskHandle) resolve(state EntityState, summary string, problems []Problem, authority resolutionAuthority) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	t.out.holdRunningPaint(t.id)
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return t
	}
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return t
	}
	if core.IsTerminalTask(st.state) {
		t.out.recordAlreadyResolvedLocked(st.name, summary)
		return t
	}
	if deniesItsOwnEffect(st, state, authority) {
		st.effectDenials++
	}
	if st.sched.submitted() && authority == byCaller && declaresSuccess(state) {
		st.proposed = &proposedOutcome{state: state, summary: summary, problems: problems}
		return t
	}
	state = st.honestOutcome(state)
	if summary != "" {
		st.summary = txt.Text(summary)
	}
	if len(problems) > 0 || len(st.problems) > 0 {
		st.problems = core.StoreProblems(st.attachEvidenceTail(state, slices.Concat(st.problems, problems)))
	}
	t.out.settleLocked(st, state)
	t.out.emitWireEventLocked(wire.EventTaskFinished, t.id, taskFinishedPayload(st))
	t.out.commitSettledLocked(st)
	return t
}

// taskFinishedPayload is the task.finished event's payload: the settled
// state, why it settled, and its Summary when it has one (ZYS-971).
func taskFinishedPayload(st *taskState) map[string]any {
	payload := map[string]any{
		"state":      string(st.state),
		"resolution": string(st.resolution),
	}
	if st.summary != "" {
		payload["summary"] = st.summary
	}
	return payload
}
