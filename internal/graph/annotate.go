// This file owns the record writes a Task's caller makes while it works: its
// activity, progress, summary and annotations. They go through the graph,
// the producer that owns the Task, so the root writes no record.

package graph

import (
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// startOnActivity moves a pending Task to Running on its first evidence of
// work and reports whether it did.
func (t *Task) startOnActivity(at time.Time) (started bool) {
	t.Rec.MarkActivity(at)
	if t.Rec.State() != record.Pending {
		return false
	}
	t.Rec.Transition(record.Running)
	return true
}

// MarkRunning moves the Task to Running.
func (t *Task) MarkRunning() { t.Rec.Transition(record.Running) }

// Doing makes text the Task's current step, as of at. A pending Task starts
// Running, with indeterminate progress; started reports that it did.
func (t *Task) Doing(text string, at time.Time) (started bool) {
	t.Rec.SetPhase(text)
	started = t.startOnActivity(at)
	if started {
		t.Rec.EnsureIndeterminateProgress()
	}
	return started
}

// Mirror is Doing for a step the Task's own output narrated: a repeat of
// the current step is no change at all. changed reports whether the step
// moved.
func (t *Task) Mirror(text string, at time.Time) (changed, started bool) {
	if !t.Rec.SetPhaseIfChanged(text) {
		return false, false
	}
	started = t.startOnActivity(at)
	if started {
		t.Rec.EnsureIndeterminateProgress()
	}
	return true, started
}

// Progress records completed of total, as of at; see record.Task.ApplyProgress.
// A rejected update records nothing else. An accepted one starts a pending
// Task Running; started reports that it did.
func (t *Task) Progress(completed, total int64, kind record.ProgressKind, at time.Time) (verdict record.ProgressVerdict, started bool) {
	verdict = t.Rec.ApplyProgress(completed, total, kind)
	if verdict == record.ProgressInvalid || verdict == record.ProgressRegressed {
		return verdict, false
	}
	return verdict, t.startOnActivity(at)
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
