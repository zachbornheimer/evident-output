package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Task is a handle for one operation with phases or progress.
type TaskHandle struct {
	out *Output
	id  string
}

// Phase sets the active phase text and starts the task if pending.
func (t *TaskHandle) Phase(text string) *TaskHandle {
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
	if isTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	t.out.setPhaseLocked(st, text)
	return t
}

// setPhaseLocked is Phase's locked body, factored out so a caller already
// holding o.mu (evo.StartPhase's declare-time phase set in taskScoped) can
// apply it without a nested lock. Callers must have already checked
// ensureOpen/isTerminalTask.
func (o *Output) setPhaseLocked(st *taskState, text string) {
	st.phase = sanitize.Text(text)
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

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// Prefer absolute Progress over Advance so retries cannot double-count.
func (t *TaskHandle) Progress(completed, total int) *TaskHandle {
	return t.setProgress(int64(completed), int64(total), Determinate)
}

// Progress64 is an advanced absolute count API for quantities outside the int range.
// Ordinary call sites should use Progress(int, int) or Bytes for byte totals.
func (t *TaskHandle) Progress64(completed, total int64) *TaskHandle {
	return t.setProgress(completed, total, Determinate)
}

// Bytes sets absolute byte progress (units and rate formatting).
func (t *TaskHandle) Bytes(completed, total int64) *TaskHandle {
	return t.setProgress(completed, total, BytesKind)
}

// Advance increments completed progress by delta.
// Advanced relative helper — prefer absolute Progress in ordinary code.
func (t *TaskHandle) Advance(delta int64) *TaskHandle {
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
	if isTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	completed := st.progress.Completed + delta
	total := st.progress.Total
	kind := st.progress.Kind
	if kind == "" || kind == Indeterminate {
		kind = Determinate
	}
	t.applyProgressLocked(st, completed, total, kind)
	return t
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
	if isTerminalTask(st.state) {
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
	if isTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	if t.applyProgressLocked(st, int64(completed), int64(total), Determinate) {
		t.out.setPhaseLocked(st, name)
	}
	return t
}

// Done resolves the task successfully.
// Optional one summary: Done("modules cached"). More than one summary panics.
func (t *TaskHandle) Done(summary ...string) *TaskHandle {
	switch len(summary) {
	case 0:
		return t.finish(Done, "", nil)
	case 1:
		return t.finish(Done, sanitize.Text(summary[0]), nil)
	default:
		panic("evo: Task.Done accepts at most one summary string")
	}
}

// Donef resolves the task with a formatted summary.
// Prefer Done("text") when there are no format directives.
func (t *TaskHandle) Donef(format string, args ...any) *TaskHandle {
	return t.finish(Done, sanitize.Text(fmt.Sprintf(format, args...)), nil)
}

// Warn resolves the task with a warning.
func (t *TaskHandle) Warn(summary string, options ...ProblemOption) *TaskHandle {
	p := applyProblemOptions(sanitize.Text(summary), options)
	return t.finish(Warning, "", []Problem{p})
}

// Warnf resolves the task with a formatted warning summary.
// Prefer Warn("text") when there are no format directives.
func (t *TaskHandle) Warnf(format string, args ...any) *TaskHandle {
	p := applyProblemOptions(sanitize.Text(fmt.Sprintf(format, args...)), nil)
	return t.finish(Warning, "", []Problem{p})
}

// Fail resolves the task as failed. This is a statement, not a fluent
// chain — Fail returns nothing, so a bare `task.Fail("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Use Failf
// to build and return a %w-wrapped error in one line.
func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	p := applyProblemOptions(sanitize.Text(summary), options)
	if t != nil {
		t.finish(Failed, sanitize.Text(summary), []Problem{p})
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
	summary, evidence := splitWrappedMessage(format, err)
	p := sanitizeProblem(Problem{Summary: summary, Detail: evidence})
	if t != nil {
		t.finish(Failed, summary, []Problem{p})
	}
	return newFailure(t, err)
}

// Block resolves the task as blocked. This is a statement, not a fluent
// chain — Block returns nothing, so a bare `task.Block("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Use
// Blockf to build and return a %w-wrapped error in one line.
func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	p := applyProblemOptions(sanitize.Text(summary), options)
	if t != nil {
		t.finish(Blocked, sanitize.Text(summary), []Problem{p})
	}
}

// Blockf resolves the task as blocked with a formatted summary and returns a
// *Failure exactly like Failf — see Failf for the fmt.Errorf %w,
// summary/evidence split, and Next/NextCommand remedy-attachment contract.
func (t *TaskHandle) Blockf(format string, args ...any) *Failure {
	err := fmt.Errorf(format, args...)
	summary, evidence := splitWrappedMessage(format, err)
	p := sanitizeProblem(Problem{Summary: summary, Detail: evidence})
	if t != nil {
		t.finish(Blocked, summary, []Problem{p})
	}
	return newFailure(t, err)
}

// Cancel resolves the task as cancelled.
func (t *TaskHandle) Cancel(reason string) *TaskHandle {
	return t.finish(Cancelled, sanitize.Text(reason), nil)
}

// Skip resolves the task as skipped.
func (t *TaskHandle) Skip(reason string) *TaskHandle {
	return t.finish(Skipped, sanitize.Text(reason), nil)
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

// NextCommand attaches a command action.
func (t *TaskHandle) NextCommand(executable string, args ...string) *TaskHandle {
	return t.Next(Command(executable, args...))
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
	if isTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return t
	}
	st.state = state
	st.phase = "" // Done clears active phase
	if summary != "" {
		st.summary = sanitize.Text(summary)
	}
	if len(problems) > 0 {
		// storeProblems: shared CSI-safe path (identical to Item).
		st.problems = storeProblems(problems)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task." + string(state), EntityID: t.id})
	// Terminal outcomes: update live ledger for collections (H.20/H.21), but do not
	// draw a live "done" frame for a standalone task right before Finish (H.17).
	if st.collection != nil {
		t.out.signalLiveLocked(true)
	} else {
		// Plain/non-TTY: stream the durable task row now so later Printf cannot
		// race above completed work (P2 / residual order contract).
		t.out.emitTaskProgressiveLocked(st)
	}
	return t
}
