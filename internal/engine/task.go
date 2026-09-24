package engine

import (
	"context"
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// TaskHandle is a handle for one operation with phases or progress.
type TaskHandle struct {
	out *Output
	id  string
}

// Doing sets the active current-step live text and starts the task if
// pending — replaces the previous text, promotes the task to Running, and
// becomes a durable line per step off-TTY. text is a printf format when args
// are present (fmt.Sprintf semantics) — one text spelling shared with
// Done/Task/Group/Reason/Skip (C6; release-gate round 6 finding 4: Confirm's
// question is the one true non-printf exception now). Named Doing, not
// Phase (P6/rename): "phase" stays the name of the run-level section header
// (StartPhase), a different concept from a task's own narrated step.
func (t *TaskHandle) Doing(text string, args ...any) *TaskHandle {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
	return t.annotate(func(st *taskState) { t.out.setPhaseLocked(st, text) })
}

// resolvedByInterrupt reports whether this state was reached by the
// interrupt sweep rather than by the caller. Narrating such a row is not
// misuse: cancellation resolves it underneath whoever was reporting on it,
// and a worker already inside its per-item step had no way to prevent the
// one straggling update that follows. Blaming the caller for the interrupt's
// own timing put a misuse warning at the top of every interrupted ledger.
func resolvedByInterrupt(state EntityState) bool {
	return state == Cancelled || state == NotStarted
}

// annotate is the one guard every non-terminal annotation verb (Doing,
// Progress, Bytes, Step, Summary, Warn, Problem, Fact) shares: under o.mu,
// it applies apply to the task's state only while the task is open. On a
// closed Output it records that misuse; on a terminal row it records
// ErrAlreadyResolved, unless the interrupt sweep resolved the row (see
// resolvedByInterrupt). It returns t so each verb can chain.
func (t *TaskHandle) annotate(apply func(st *taskState)) *TaskHandle {
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
		if !resolvedByInterrupt(st.state) {
			t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		}
		return t
	}
	apply(st)
	return t
}

// setLiveOnlyPhase updates the task's phase text through setLiveOnlyPhaseLocked
// — the shared entry point for every phase source that is NOT the caller's
// own narrated beat: Writer's per-line mirror of a talkative child's raw
// output, and Step's current-item name. Off-TTY, an explicit TaskHandle.Doing call
// still forces its own durable row (the P10 contract: the one line the
// caller asked to see); this path never does — a child's full output already
// has one durable home, the evidence ring (and its failure-path DetailTail),
// so a row per mirrored line would just repeat it (release-gate round 9
// finding 4). Step is the same shape: Isolated+Plain must not stream a
// durable line per unique item name.
func (t *TaskHandle) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

// setPhaseLocked is Phase's locked body, factored out so a caller already
// holding o.mu (evo.StartPhase's declare-time phase set in taskScoped) can
// apply it without a nested lock. Callers must have already checked
// ensureOpen/isTerminalTask.
func (o *Output) setPhaseLocked(st *taskState, text string) {
	st.phase = txt.Text(text)
	st.activityAt = o.cfg.clock.Now()
	if st.state == Pending {
		o.promoteRunningLocked(st)
		if st.progress.Kind == "" {
			st.progress.Kind = Indeterminate
		}
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
	o.emitTaskRunningProgressiveLocked(st, triggerPhase)
}

// setLiveOnlyPhaseLocked is the shared locked body behind every phase
// source that is not the caller's own narrated beat (see setLiveOnlyPhase):
// the same state transition as setPhaseLocked (promotion, activity clock,
// live redraw signal), but it never forces its own durable line in plain
// mode. A talkative child's mirrored output line (Writer) already has one
// durable home, the evidence ring, so it gets no plain-mode row per line
// (release-gate round 9 finding 4); Step's current-item name is live
// status for the same reason, never a durable line per item.
func (o *Output) setLiveOnlyPhaseLocked(st *taskState, text string) {
	text = txt.Text(text)
	// Identical live-only phase is a no-op: Writer leftover/repeated lines
	// must not bump, emit task.phase_changed, or force a live paint.
	if text == st.phase {
		return
	}
	st.phase = text
	st.activityAt = o.cfg.clock.Now()
	if st.state == Pending {
		o.promoteRunningLocked(st)
		if st.progress.Kind == "" {
			st.progress.Kind = Indeterminate
		}
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
}

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// Prefer absolute Progress over Advance so retries cannot double-count.
func (t *TaskHandle) Progress(completed, total int) *TaskHandle {
	return t.setProgress(int64(completed), int64(total), Determinate)
}

// Bytes sets absolute byte progress (units and rate formatting).
func (t *TaskHandle) Bytes(completed, total int64) *TaskHandle {
	return t.setProgress(completed, total, BytesKind)
}

func (t *TaskHandle) setProgress(completed, total int64, kind ProgressKind) *TaskHandle {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied — false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse instead, letting a caller like Step skip a paired update
// (e.g. Phase) that would otherwise describe a progress change that never
// happened.
func (t *TaskHandle) applyProgressLocked(st *taskState, completed, total int64, kind ProgressKind) bool {
	if completed < 0 || total < 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	}
	if total == 0 && completed != 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	}
	if completed > total && total > 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	}
	// Regression and sealing guards apply only while re-reporting the same
	// measurement kind (Determinate or Bytes); switching kind (e.g. Progress
	// then Bytes) is a deliberate re-declaration and resets both freely.
	if st.state == Running && st.progress.Kind != Indeterminate && st.progress.Total > 0 && kind == st.progress.Kind {
		if completed < st.progress.Completed {
			t.out.recordMisuse(ErrProgressRegression)
			return false
		}
		// Sealed total: once a nonzero total is reported for this kind, it
		// cannot change. Retry-safety depends on the denominator staying put.
		if total != st.progress.Total {
			t.out.recordMisuse(ErrInvalidProgress)
			return false
		}
	}
	st.progress = Progress{Kind: kind, Completed: completed, Total: total}
	st.activityAt = t.out.cfg.clock.Now()
	if st.state == Pending {
		t.out.promoteRunningLocked(st)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.signalLiveLocked(false)
	t.out.emitTaskRunningProgressiveLocked(st, triggerProgress)
	return true
}

// Step sets absolute progress and the current item name together under
// one lock, so a concurrent worker can never observe one goroutine's
// count paired with another goroutine's name — the exact interleaving
// two separate Progress(...) + Doing(...) calls (two separate locks) allow.
// The name is live-only: Isolated+Plain does not stream a durable phase
// line per unique name (thinned progress milestones still emit). Doing
// remains the durable narrated-beat path.
func (t *TaskHandle) Step(completed, total int, name string) *TaskHandle {
	return t.annotate(func(st *taskState) {
		if t.applyProgressLocked(st, int64(completed), int64(total), Determinate) {
			t.out.setLiveOnlyPhaseLocked(st, name)
		}
	})
}

// succeed resolves the task Done with summary: the engine's synchronous
// success verb for library-owned rows (Confirm's gate) and engine tests.
// The public TaskHandle has no equivalent since 1.1: callers resolve
// through Define, and Summary carries their result text.
func (t *TaskHandle) succeed(summary string) {
	t.finish(Done, txt.Text(summary), nil)
}

// Warn accumulates a non-blocking warning on the task. It takes the same
// structured ProblemOption metadata as Problem/Fail/Block
// (Detail/Code/On/Location/Next). It never resolves the task ("warnings
// annotate lifecycle; they do not replace it"), so call it any number of
// times before the task resolves; a warned task that never resolves
// auto-resolves Done at Finish. It returns t only so a call can chain, and
// summary is never Sprintf-formatted.
func (t *TaskHandle) Warn(summary string, opts ...ProblemOption) *TaskHandle {
	p := applyProblemOptions(txt.Text(summary), opts)
	return t.annotate(func(st *taskState) {
		st.warnings = append(st.warnings, p)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.warned", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventWarningRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Summary sets one sanitized line of result text for the task's terminal
// row: last call wins, empty clears it. It never resolves the task (Define
// or the Evo-native operation outcome does), and it is not live activity
// (Doing/Progress/Step/Bytes own the Running row). It is the same field
// TaskSnapshot.Summary and JSON/JSONL "summary" project. Calling it after
// the task resolved is misuse unless the interrupt sweep resolved it (see
// annotate). GroupHandle.Summary is the same shape one level up.
func (t *TaskHandle) Summary(text string) *TaskHandle {
	return t.annotate(func(st *taskState) {
		st.summary = txt.Text(text)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.summary_set", EntityID: t.id})
		t.out.signalLiveLocked(true)
	})
}

// Problem appends one blocking Problem to the task without resolving it,
// so a Define callback may accumulate many structured findings: one owning
// Task retains zero, one, or many Problems instead of a caller-invented
// Task per finding or one newline-delimited error string. The Problem is
// part of the Task from the moment it is recorded (Snapshot, live render,
// JSON), order preserved, nothing dropped. A Task holding a Problem can
// never settle success-class: Done, Skipped, or Finish's amnesty for an
// unresolved Task all settle Failed instead (see honestOutcome). Problem
// returns t so calls chain: task.Problem(...).Problem(...).
func (t *TaskHandle) Problem(summary string, opts ...ProblemOption) *TaskHandle {
	p := applyProblemOptions(txt.Text(summary), opts)
	return t.annotate(func(st *taskState) {
		st.problems = append(st.problems, core.StoreProblems([]Problem{p})...)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.problem_recorded", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventProblemRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Fact accumulates a discovered name/value annotation on the task — info
// severity, Warn's non-terminal sibling (user-13-problems.md Problem 8:
// "Tasks are work. Facts are information."). Renders as a dim "name  value"
// line, inline when it is the task's only annotation, nested otherwise.
// Like Warn, this is a statement (no return value) and never resolves the
// task — call it any number of times before the task's terminal verb.
func (t *TaskHandle) Fact(name, value string) *TaskHandle {
	f := core.SanitizeFact(FactRecord{Name: txt.Text(name), Value: txt.Text(value)})
	return t.annotate(func(st *taskState) {
		st.facts = append(st.facts, f)
		t.out.bumpLocked()
		t.out.emitWireEventLocked(wire.EventFactRecorded, t.id, wire.ToFactDoc(f).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Fail resolves the task as failed. This is a statement, not a fluent
// chain — Fail returns nothing, so a bare `task.Fail("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Use Failf
// to build and return a %w-wrapped error in one line.
func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	p := applyProblemOptions(txt.Text(summary), options)
	if t != nil {
		t.finish(Failed, txt.Text(summary), []Problem{p})
	}
}

// Failf resolves the task as failed with a formatted summary and returns a
// *Failure so a call site can `return` it directly:
// `return task.Failf("validate policy manifest: %w", err)`, and attach a
// remedy in the same statement: `.Next(evo.Label("..."))`. fmt.Errorf
// semantics: %w wraps its argument so errors.Is/As still reach it. See
// splitWrappedMessage for how a trailing ": %w"/", %w" splits the formatted
// text into the rendered summary and evidence line.
func (t *TaskHandle) Failf(format string, args ...any) *Failure {
	err := fmt.Errorf(format, args...)
	summary, evidence := core.SplitWrappedMessage(format, err)
	problem := Problem{Summary: summary, Detail: evidence}
	t.attachRetainedEvidenceTail(&problem)
	p := core.SanitizeProblem(problem)
	if t != nil {
		t.finish(Failed, summary, []Problem{p})
	}
	return newFailure(t, err)
}

// attachRetainedEvidenceTail attaches the task's own retained evidence
// (Writer/PhaseWriter/evidence() capture) as the Problem's
// EvidenceTail, the same precedence Evidence.DetailTail() already
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
	t.evidence().DetailTail().applyProblem(p)
}

// Block resolves the task as blocked. This is a statement, not a fluent
// chain — Block returns nothing, so a bare `task.Block("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Use
// Blockf to build and return a %w-wrapped error in one line.
func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	p := applyProblemOptions(txt.Text(summary), options)
	if t != nil {
		t.finish(Blocked, txt.Text(summary), []Problem{p})
	}
}

// Blockf resolves the task as blocked with a formatted summary and returns a
// *Failure exactly like Failf — see Failf for the fmt.Errorf %w,
// summary/evidence split, and Next/NextCommand remedy-attachment contract.
func (t *TaskHandle) Blockf(format string, args ...any) *Failure {
	err := fmt.Errorf(format, args...)
	summary, evidence := core.SplitWrappedMessage(format, err)
	problem := Problem{Summary: summary, Detail: evidence}
	t.attachRetainedEvidenceTail(&problem)
	p := core.SanitizeProblem(problem)
	if t != nil {
		t.finish(Blocked, summary, []Problem{p})
	}
	return newFailure(t, err)
}

// Cancel resolves the task as cancelled.
func (t *TaskHandle) Cancel(reason string) {
	t.finish(Cancelled, txt.Text(reason), nil)
}

// Skip resolves the task as skipped. reason is a printf format when args are
// present (fmt.Sprintf semantics) — one text spelling shared with
// Done/Task/Group/Reason/Phase (C6; release-gate round 6 finding 4).
func (t *TaskHandle) skip(reason string, args ...any) *TaskHandle {
	if len(args) > 0 {
		reason = fmt.Sprintf(reason, args...)
	}
	return t.finish(Skipped, txt.Text(reason), nil)
}

// Next attaches actions.
func (t *TaskHandle) Next(actions ...Action) *TaskHandle {
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return t
	}
	if t.out.finishing || t.out.finished || t.out.closed {
		t.out.recordMisuse(ErrClosed)
		return t
	}
	st.actions = append(st.actions, cloneActions(actions)...)
	t.out.bumpLocked()
	return t
}

// NextCommand attaches a command action. args names a foreign tool's own
// executable explicitly — the common case, since most remedies point at a
// different tool than the one running right now.
func (t *TaskHandle) NextCommand(executable string, args ...string) *TaskHandle {
	return t.Next(Command(executable, args...))
}

// NextSelf attaches a command action that re-runs the caller's own binary
// with args — a self-referencing remedy ("rerun with --apply") that doesn't
// restate which binary to run (I6). Uses the same identity source as
// Confirm's PolicyFlag / I2's Failf fallback: Config.Title when set, else
// the binary's own basename. Use NextCommand instead when the remedy is a
// different (foreign) tool.
func (t *TaskHandle) nextSelf(args ...string) *TaskHandle {
	return t.NextCommand(t.out.policySourceName(), args...)
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

// Context reports the cancellation signal this task's work runs under — the
// run's own (see Output.Context), so a Define or Effect callback
// doing I/O selects on it and stops when the run is interrupted.
func (t *TaskHandle) Context() context.Context {
	if t == nil {
		return context.Background()
	}
	return t.out.Context()
}

// Snapshot returns the task snapshot.
func (t *TaskHandle) Snapshot() TaskSnapshot {
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return TaskSnapshot{ID: t.id, State: Pending, Progress: Progress{Kind: Indeterminate}}
	}
	return st.snapshot()
}

// finish is the caller-facing resolution path — every terminal verb
// (Done/Fail/Block/Cancel/Skip) a program writes by hand.
//
// Once a task is submitted, the scheduler owns the verdict on its work: only
// the callback's own return value says whether the work succeeded. So a
// hand-written Done/Skipped on a submitted task is a *proposal*, not a
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
	t.out.emitWireEventLocked(wire.EventTaskFinished, t.id, map[string]any{
		"state":      string(state),
		"resolution": string(st.resolution),
	})
	t.out.commitSettledLocked(st)
	return t
}

// honestOutcome is the one rule between a Task's blocking evidence and its
// terminal state: a Task holding any Problem cannot settle success-class,
// so a Done or Skipped claim over one settles Failed. settleLocked applies
// it to every path that ends a Task.
func (st *taskState) honestOutcome(state EntityState) EntityState {
	if declaresSuccess(state) && len(st.problems) > 0 {
		return Failed
	}
	return state
}

// attachEvidenceTail gives a Failed or Blocked row's Problems the capture
// tail the Task already gathered, so the detail a caller collected through
// evidence()/PhaseWriter() needs no opt-in (beginner-2). A Problem with its
// own Detail or EvidenceTail keeps it; that also avoids re-entering the
// redactor lock this resolution already holds for a pending tail.
func (st *taskState) attachEvidenceTail(state EntityState, problems []Problem) []Problem {
	if (state != Failed && state != Blocked) || st.evidence == nil || st.evidence.Empty() {
		return problems
	}
	for i := range problems {
		if problems[i].Detail == "" && problems[i].EvidenceTail == "" {
			problems[i].Detail = st.evidence.detailText()
		}
	}
	return problems
}

// commitSettledLocked writes a settled Task where its readers find it. A
// Group or Sequence child repaints the live ledger; a standalone Task
// commits its row and its own File/Exec ledger rows to scrollback at once,
// so later Printf/Confirm output can never land above finished work. A
// Done Task commits its manifest record; Failed, Blocked, and Cancelled
// never do (spec §8.2/§11.3).
func (o *Output) commitSettledLocked(st *taskState) {
	if st.collection != nil {
		o.signalLiveLocked(true)
	} else {
		o.commitResolvedTaskLocked(st.id)
		o.commitNamedEffectsLocked(st.id)
	}
	if st.state != Done {
		return
	}
	runCtx := o.ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	o.commitManifestTaskLocked(runCtx, st.id)
}
