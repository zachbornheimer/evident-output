package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Task is a handle for one operation with phases or progress.
type Task struct {
	out *Output
	id  string
}

// Phase sets the active phase text and starts the task if pending.
func (t *Task) Phase(text string) *Task {
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
		t.out.recordMisuse(ErrAlreadyResolved)
		return t
	}
	st.phase = sanitize.Text(text)
	if st.state == Pending {
		st.state = Running
		if st.progress.Kind == "" {
			st.progress.Kind = Indeterminate
		}
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task.phase_changed", EntityID: t.id})
	t.out.signalLiveLocked(true)
	return t
}

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// For values outside int range (tests / huge counters) use Progress64.
func (t *Task) Progress(completed, total int) *Task {
	return t.setProgress(int64(completed), int64(total), Determinate)
}

// Progress64 sets absolute completed/total progress using int64 quantities.
func (t *Task) Progress64(completed, total int64) *Task {
	return t.setProgress(completed, total, Determinate)
}

// Bytes sets absolute byte progress.
func (t *Task) Bytes(completed, total int64) *Task {
	return t.setProgress(completed, total, BytesKind)
}

// Advance increments completed progress by delta.
func (t *Task) Advance(delta int64) *Task {
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
		t.out.recordMisuse(ErrAlreadyResolved)
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

func (t *Task) setProgress(completed, total int64, kind ProgressKind) *Task {
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
		t.out.recordMisuse(ErrAlreadyResolved)
		return t
	}
	t.applyProgressLocked(st, completed, total, kind)
	return t
}

func (t *Task) applyProgressLocked(st *taskState, completed, total int64, kind ProgressKind) {
	if completed < 0 || total < 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return
	}
	if total == 0 && completed != 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return
	}
	if completed > total && total > 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return
	}
	// Regression: completed decreases without restart.
	if st.state == Running && st.progress.Kind != Indeterminate && st.progress.Total > 0 {
		if completed < st.progress.Completed {
			t.out.recordMisuse(ErrProgressRegression)
			return
		}
		if total < st.progress.Total && total < completed {
			t.out.recordMisuse(ErrInvalidProgress)
			return
		}
		// total may not decrease below completed
		if total < st.progress.Completed {
			t.out.recordMisuse(ErrInvalidProgress)
			return
		}
	}
	st.progress = Progress{Kind: kind, Completed: completed, Total: total}
	if st.state == Pending {
		st.state = Running
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.signalLiveLocked(false)
}

// Done resolves the task successfully.
// Optional one summary: Done("modules cached"). More than one summary panics.
func (t *Task) Done(summary ...string) *Task {
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
func (t *Task) Donef(format string, args ...any) *Task {
	return t.finish(Done, sanitize.Text(fmt.Sprintf(format, args...)), nil)
}

// Warn resolves the task with a warning.
func (t *Task) Warn(summary string, options ...ProblemOption) *Task {
	p := applyProblemOptions(sanitize.Text(summary), options)
	return t.finish(Warning, "", []Problem{p})
}

// Fail resolves the task as failed.
func (t *Task) Fail(summary string, options ...ProblemOption) *Task {
	p := applyProblemOptions(sanitize.Text(summary), options)
	return t.finish(Failed, sanitize.Text(summary), []Problem{p})
}

// Cancel resolves the task as cancelled.
func (t *Task) Cancel(reason string) *Task {
	return t.finish(Cancelled, sanitize.Text(reason), nil)
}

// Skip resolves the task as skipped.
func (t *Task) Skip(reason string) *Task {
	return t.finish(Skipped, sanitize.Text(reason), nil)
}

// Next attaches actions.
func (t *Task) Next(actions ...Action) *Task {
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
func (t *Task) NextCommand(executable string, args ...string) *Task {
	return t.Next(Command(executable, args...))
}

// Snapshot returns the task snapshot.
func (t *Task) Snapshot() TaskSnapshot {
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return TaskSnapshot{ID: t.id, State: Pending, Progress: Progress{Kind: Indeterminate}}
	}
	return st.snapshot()
}

func (t *Task) finish(state EntityState, summary string, problems []Problem) *Task {
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
		t.out.recordMisuse(ErrAlreadyResolved)
		return t
	}
	st.state = state
	st.phase = "" // Done clears active phase
	if summary != "" {
		st.summary = sanitize.Text(summary)
	}
	if len(problems) > 0 {
		st.problems = cloneProblems(problems)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task." + string(state), EntityID: t.id})
	// Terminal outcomes: update live ledger for collections (H.20/H.21), but do not
	// draw a live "done" frame for a standalone task right before Finish (H.17).
	if st.collection != nil {
		t.out.signalLiveLocked(true)
	}
	return t
}
