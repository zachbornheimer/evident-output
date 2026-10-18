// This file owns the record writes a Task's caller makes while it works: its
// activity, progress, summary and annotations. They go through the graph,
// the producer that owns the Task, so the root writes no record.

package graph

import (
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// MarkRunning moves the Task to Running: the scheduler calls it as it starts
// the Task's callback, and nothing else makes a Task Running. A step the Task
// narrated while it waited is held on the record and shows from here, with
// the indeterminate progress a step without a count implies.
func (t *Task) MarkRunning() {
	t.Rec.Transition(record.Running)
	if t.Rec.Phase() != "" {
		t.Rec.EnsureIndeterminateProgress()
	}
}

// Doing makes text the Task's current step, as of at. It never starts the
// Task: a step set before the callback runs is held until MarkRunning. A
// Task already Running gets indeterminate progress if it has none.
func (t *Task) Doing(text string, at time.Time) {
	t.Rec.SetPhase(text)
	t.Rec.MarkActivity(at)
	t.ensureIndeterminateWhileRunning()
}

// Mirror is Doing for a step the Task's own output narrated: a repeat of
// the current step is no change at all. changed reports whether the step
// moved.
func (t *Task) Mirror(text string, at time.Time) (changed bool) {
	if !t.Rec.SetPhaseIfChanged(text) {
		return false
	}
	t.Rec.MarkActivity(at)
	t.ensureIndeterminateWhileRunning()
	return true
}

// Progress records completed of total, as of at; see record.Task.ApplyProgress.
// A rejected update records nothing else. An accepted one never starts the
// Task: a count reported before the callback runs is held until MarkRunning.
func (t *Task) Progress(completed, total int64, kind record.ProgressKind, at time.Time) record.ProgressVerdict {
	verdict := t.Rec.ApplyProgress(completed, total, kind)
	if verdict == record.ProgressApplied {
		t.Rec.MarkActivity(at)
	}
	return verdict
}

func (t *Task) ensureIndeterminateWhileRunning() {
	if t.Rec.State() == record.Running {
		t.Rec.EnsureIndeterminateProgress()
	}
}

// Summarize sets the Task's one line of result text; empty clears it.
func (t *Task) Summarize(text string) { t.Rec.SetSummary(text) }

// Report records Problems that do not end the Task.
func (t *Task) Report(problems ...record.Problem) { t.Rec.AppendProblems(problems...) }

// Note records a discovered name/value Fact.
func (t *Task) Note(f record.Fact) { t.Rec.AppendFact(f) }

// Recommend records a next step the reader can take.
func (t *Task) Recommend(a record.Action) { t.Rec.AppendAction(a) }

// Summarize sets the container's own result text.
func (c *Container) Summarize(text string) { c.Rec.SetSummary(text) }

// Capture is the Task's retained process output, made by newCapture on
// first use; see record.Task.CaptureOr.
func (t *Task) Capture(newCapture func() *record.Capture) *record.Capture {
	return t.Rec.CaptureOr(newCapture)
}
