package evo

import (
	"io"

	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// writerIsCharDevice is the TTY-detection facade construction uses.
func writerIsCharDevice(w io.Writer) bool { return terminal.WriterIsCharDevice(w) }

// sameTerminalDevice reports whether a and b name one physical character device; construction uses it to
// detect Diagnostics sharing the live region's terminal, which writer identity alone misses.
func sameTerminalDevice(a, b io.Writer) bool { return terminal.SameDevice(a, b) }

func (r *Run) Summary(text string) *Run {
	if r != nil {
		r.graph.Root().Summarize(text)
	}
	r.subject(text)
	return r
}

// Progress sets absolute completed/total progress; use Bytes for byte
// quantities. Prefer it over Advance so retries cannot double-count.
func (t *Task) Progress(completed, total int) *Task {
	return t.setProgress(int64(completed), int64(total), determinate)
}

func (t *Task) setProgress(completed, total int64, kind progressKind) *Task {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied; false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse.
func (t *Task) applyProgressLocked(st *taskState, completed, total int64, kind progressKind) bool {
	verdict := st.node.Progress(completed, total, kind, t.out.cfg.clock.Now())
	switch verdict {
	case record.ProgressInvalid:
		t.out.recordMisuse(errInvalidProgress)
		return false
	case record.ProgressRegressed:
		t.out.recordMisuse(errProgressRegression)
		return false
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(recEvent{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.show.Live.Signal(false)
	t.out.show.Stream.Progressed(st.node.TaskID(), t.out.cfg.clock.Now())
	return true
}

// Writer returns a line-buffered io.Writer for a talkative child process: each complete line becomes the Task's
// live current item, and every byte is kept so a failure shows the output that explains it. Off a terminal,
// lines never become durable rows. Safe for concurrent use. See the Task.Writer example.
func (t *Task) Writer() io.Writer {
	if t == nil || t.out == nil {
		return io.Discard
	}
	return txt.NewChildWriter(t.capture(), t.appendLiveTail, t.setLiveOnlyPhase)
}
