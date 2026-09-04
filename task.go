package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Task is a handle for one operation with phases or progress.
type TaskHandle struct {
	out *Output
	id  string
}

// Phase sets the active phase text and starts the task if pending. text is a
// printf format when args are present (fmt.Sprintf semantics) — one text
// spelling shared with Done/Task/Group/Reason/Skip (C6; release-gate round 6
// finding 4: Confirm's question is the one true non-printf exception now).
func (t *TaskHandle) Phase(text string, args ...any) *TaskHandle {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
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
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	t.out.setPhaseLocked(st, text)
	return t
}

// setLiveOnlyPhase updates the task's phase text through setLiveOnlyPhaseLocked
// — the shared entry point for every phase source that is NOT the caller's
// own narrated beat: Each's per-item courtesy default, and PhaseWriter's (and
// through it, Task.Run's) per-line mirror of a talkative child's raw output.
// Off-TTY, an explicit TaskHandle.Phase call still forces its own durable row
// (the P10 contract: the one line the caller asked to see); this path never
// does — a child's full output already has one durable home, the Evidence
// ring (and its failure-path DetailTail), so a row per mirrored line would
// just repeat it (release-gate round 9 finding 4).
func (t *TaskHandle) setLiveOnlyPhase(text string) {
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return
	}
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return
	}
	if core.IsTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return
	}
	t.out.setLiveOnlyPhaseLocked(st, text)
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
// mode. Each's bare item name is a courtesy default, not a caller-declared
// phase — if the loop body sets its own Phase before the next paint, that
// call's own emission carries the current text once, instead of the reader
// seeing the item name and the body's phase as two separate redundant lines
// (beginner-10). A talkative child's mirrored output line is the same
// shape: the Evidence ring is its one durable home, not a plain-mode row
// per line (release-gate round 9 finding 4).
func (o *Output) setLiveOnlyPhaseLocked(st *taskState, text string) {
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
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	t.applyProgressLocked(st, completed, total, kind)
	return t
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

// Step sets absolute progress and phase text together under one lock
// acquisition, so a concurrent worker can never observe one goroutine's
// count paired with another goroutine's phase name — the exact interleaving
// two separate Progress(...) + Phase(...) calls (two separate locks) allow.
func (t *TaskHandle) Step(completed, total int, name string) *TaskHandle {
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
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	if t.applyProgressLocked(st, int64(completed), int64(total), Determinate) {
		t.out.setPhaseLocked(st, name)
	}
	return t
}

// Done resolves the task successfully, with no summary (Done()), a literal
// one (Done("modules cached")), or a printf-formatted one
// (Done("%d packages", 18), fmt.Sprintf semantics) — one text spelling
// shared with Task/Group/Reason/Warn (C6), including the same "no args
// leaves text untouched" rule that keeps a literal "%" safe. A non-string
// first argument is misuse (ErrInvalidConfig): Done's format position is
// still meant to be a caller-written string, not an accidental value.
func (t *TaskHandle) Done(args ...any) *TaskHandle {
	summary, ok := formatSummaryArgs(args)
	if !ok {
		t.out.recordMisuse(ErrInvalidConfig)
		return t
	}
	return t.finish(Done, txt.Text(summary), nil)
}

// Unchanged resolves the task successfully, explicitly marking "checked,
// nothing needed to change" — distinct from an ordinary Done's generic
// verdict (I7). A run made entirely of Unchanged tasks (no Changes/Plan
// records, nothing Failed/Blocked/Cancelled/Warning) concludes
// StateUnchanged instead of the StateReady an ordinary Done gets. Same
// no-args/literal/printf-formatted shape as Done (C6).
func (t *TaskHandle) Unchanged(args ...any) *TaskHandle {
	summary, ok := formatSummaryArgs(args)
	if !ok {
		t.out.recordMisuse(ErrInvalidConfig)
		return t
	}
	return t.finishTagged(Done, txt.Text(summary), nil, true)
}

// formatSummaryArgs implements Done/Unchanged's no-args/literal/printf-
// formatted shape (C6): zero args is no summary; one string arg is a
// literal (never passed through Sprintf, so a caller's own "%" stays
// intact); two or more requires args[0] to be a printf format string,
// applied to the rest via fmt.Sprintf. ok is false only when a non-string
// first argument is given — a genuine caller mistake, not a valid shape.
func formatSummaryArgs(args []any) (summary string, ok bool) {
	if len(args) == 0 {
		return "", true
	}
	format, isString := args[0].(string)
	if !isString {
		return "", false
	}
	if len(args) == 1 {
		return format, true
	}
	return fmt.Sprintf(format, args[1:]...), true
}

// Warn resolves the task with a warning. This is a statement, not a fluent
// chain — Warn returns nothing, so a bare `task.Warn("summary")` is
// errcheck-clean, matching Fail/Block (beginner-9: doc.go's no-fluent
// promise). The message gets the same summary placement as Fail/Block (the
// ⚠ row itself carries it), and the same de-echo as Fail/Block drops the
// redundant problem row when there's no Detail beyond it (beginner-3).
// summary is a printf format when fmt args are present — one text spelling
// shared with Done/Task/Group/Reason (C6); evo.Detail(...) and other
// ProblemOptions may be mixed into args in any position and still apply.
func (t *TaskHandle) Warn(summary string, args ...any) {
	formatted, opts := formatWarnArgs(summary, args)
	p := applyProblemOptions(txt.Text(formatted), opts)
	t.finish(Warning, formatted, []Problem{p})
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

// attachRetainedEvidenceTail attaches the task's own retained Evidence
// (task.Run(cmd)/PhaseWriter/Evidence() capture) as the Problem's
// EvidenceTail, the same precedence Evidence.DetailTail() already
// documents: an existing Detail line — here, Failf/Blockf's own
// wrapped-error text — still renders as the primary line, and the retained
// evidence appends underneath rather than being silently dropped
// (beginner-gate-2 finding 3). Failf/Blockf accept no ProblemOptions, so
// this is the only way their call sites ever see the proof task.Run already
// captured; a bare Fail/Block with no Detail still gets its own auto-attach
// from finishTagged, unaffected by this.
func (t *TaskHandle) attachRetainedEvidenceTail(p *Problem) {
	if t == nil {
		return
	}
	t.Evidence().DetailTail().applyProblem(p)
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
func (t *TaskHandle) Cancel(reason string) *TaskHandle {
	return t.finish(Cancelled, txt.Text(reason), nil)
}

// Skip resolves the task as skipped. reason is a printf format when args are
// present (fmt.Sprintf semantics) — one text spelling shared with
// Done/Task/Group/Reason/Phase (C6; release-gate round 6 finding 4).
func (t *TaskHandle) Skip(reason string, args ...any) *TaskHandle {
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
func (t *TaskHandle) NextSelf(args ...string) *TaskHandle {
	return t.NextCommand(t.out.policySourceName(), args...)
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

func (t *TaskHandle) finish(state EntityState, summary string, problems []Problem) *TaskHandle {
	return t.finishTagged(state, summary, problems, false)
}

// finishTagged is finish's body, with an extra unchanged tag Task.Unchanged/
// Unchangedf set (I7) — a run made entirely of Unchanged tasks concludes
// StateUnchanged instead of the generic StateReady an ordinary Done gets
// (inferConclusion). Never called with unchanged=true for any state other
// than Done — Unchanged is a Done-family resolution, not a new glyph.
func (t *TaskHandle) finishTagged(state EntityState, summary string, problems []Problem, unchanged bool) *TaskHandle {
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
	st.state = state
	st.unchanged = unchanged
	st.phase = "" // Done clears active phase
	if summary != "" {
		st.summary = txt.Text(summary)
	}
	if len(problems) > 0 {
		// Fail/Block with a non-empty evidence ring and no explicit Detail or
		// EvidenceTail auto-attach the capture tail (beginner-2) — the
		// evidence a caller already gathered via Evidence()/PhaseWriter() is
		// exactly the detail a Fail/Block row needs, so DetailTail is no
		// longer an opt-in step a caller has to remember. Skipping when
		// EvidenceTail is already set (an explicit DetailTail() ran as a
		// ProblemOption before finishTagged's lock was taken) avoids a
		// duplicate render and, more importantly, avoids re-entering
		// st.evidence.detailText — which, for a pending (unterminated-line)
		// tail, calls back into this Output's redactor lock — while this
		// method already holds that same lock.
		if (state == Failed || state == Blocked) && st.evidence != nil && !st.evidence.Empty() {
			for i := range problems {
				if problems[i].Detail == "" && problems[i].EvidenceTail == "" {
					problems[i].Detail = st.evidence.detailText()
				}
			}
		}
		// storeProblems: shared CSI-safe path (identical to Item).
		st.problems = core.StoreProblems(problems)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task." + string(state), EntityID: t.id})
	// Terminal outcomes: update live ledger for collections (H.20/H.21). A
	// standalone task commits its own row to durable scrollback right now,
	// interactive or not, so a later Printf/Println/Confirm can never race
	// above already-resolved work (P2 / residual order contract;
	// release-gate round 5 finding 3 — see commitResolvedTaskLocked).
	if st.collection != nil {
		t.out.signalLiveLocked(true)
	} else {
		t.out.commitResolvedTaskLocked(st.id)
	}
	return t
}
