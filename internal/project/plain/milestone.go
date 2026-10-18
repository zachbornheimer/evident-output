package plain

import (
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// progressMilestones is how many roughly even steps a determinate total is
// cut into for plain streaming (beginner-8): a line per increment would
// flood a CI log, one line and then silence until Done would say nothing.
const progressMilestones = 10

// firstNarrationGap is the least time between two Doing lines of one Task.
// Each Doing line doubles the gap up to heartbeatInterval, so a loop naming
// a thousand items writes a handful of lines, never a line per name.
const firstNarrationGap = time.Second

// mark is what plain streaming already wrote for a Running Task's
// narration. Its first line, from either verb, is always durable; after that
// Progress thins by count milestones and Doing by elapsed time.
type mark struct {
	// progressStarted is false until the first progress tick streamed;
	// progressEmitted is the last completed value streamed.
	progressStarted bool
	progressEmitted int64
	// started is true once any narration line streamed; lastLine is when the
	// latest one did; gap is how long a Doing line must wait after it.
	started  bool
	lastLine time.Time
	gap      time.Duration
	// pairs is true once the Task was seen calling Doing right after a
	// Progress tick (Progress(n, total).Doing(item), the documented form),
	// so a milestone is owed until that Doing names its item; owed is that
	// wait. afterProgress is set by each tick and cleared by the next Doing.
	pairs, owed, afterProgress bool
}

// crossesMilestone reports whether completed of total crosses a milestone
// since the last streamed tick. The first and the final tick always do
// (beginner-8's "always a final n/n"). The step rounds up, so a total of 13
// or 29 still streams about ten lines, never one per item.
func (m mark) crossesMilestone(completed, total int64) bool {
	if !m.progressStarted {
		return true
	}
	if completed == m.progressEmitted {
		return false
	}
	if total <= 0 || completed >= total {
		return true
	}
	step := max((total+progressMilestones-1)/progressMilestones, 1)
	return completed/step != m.progressEmitted/step
}

// doingDue reports whether a Doing call at now earns a line: the first
// narration of the Task always does. Once Progress streams, its milestones
// own the thinning and Doing adds no line of its own, since every milestone
// already carries the latest text.
func (m mark) doingDue(now time.Time) bool {
	if !m.started {
		return true
	}
	return !m.progressStarted && now.Sub(m.lastLine) >= m.gap
}

// Progressed streams the Task id's progress as a durable line when no live
// region shows it (evo-rec.md Problem 10), once per milestone. A Task inside
// a collection streams too: "never silent between the first line and Done"
// does not depend on where the Running Task sits in the tree.
func (s *Stream) Progressed(id record.TaskID, now time.Time) {
	r := s.runningRow(id)
	if r == nil {
		return
	}
	if r.mark.afterProgress {
		// The Doing that was to name the last tick's item never came: this
		// Task does not pair them, so no milestone waits for one.
		r.mark.pairs, r.mark.owed = false, false
	}
	r.mark.afterProgress = true
	truth := r.Rec.Truth()
	if !r.mark.crossesMilestone(truth.Progress.Completed, truth.Progress.Total) {
		return
	}
	r.mark.progressStarted = true
	r.mark.progressEmitted = truth.Progress.Completed
	if r.mark.pairs {
		r.mark.owed = true
		return
	}
	s.streamLatest(r, now)
}

// Narrated streams the Task id's current item (Doing) as a durable line when
// no live region shows it: the first call at once, so a piped reader sees
// the Task start, then milestones that carry the latest text. The 30 s
// heartbeat covers any silence after the last one.
func (s *Stream) Narrated(id record.TaskID, now time.Time) {
	r := s.runningRow(id)
	if r == nil {
		return
	}
	r.mark.pairs = r.mark.pairs || r.mark.afterProgress
	r.mark.afterProgress = false
	if r.mark.owed {
		s.streamLatest(r, now)
		return
	}
	if !r.mark.doingDue(now) {
		return
	}
	r.mark.gap = min(max(r.mark.gap*2, firstNarrationGap), heartbeatInterval)
	s.streamLatest(r, now)
}

// runningRow is the row of Task id when plain streaming owes it narration:
// it is Running and no live region shows it. Nil otherwise.
func (s *Stream) runningRow(id record.TaskID) *row {
	r := s.rows[id]
	if r == nil || s.cfg.Region.Interactive() || r.Rec.Truth().State != record.Running {
		return nil
	}
	return r
}

// flushOwed writes the milestone r still owes, with the item current now.
// A Task that settles right after its last tick has no Doing left to name.
func (s *Stream) flushOwed(r *row) {
	if r.mark.owed {
		s.streamLatest(r, s.cfg.Clock.Now())
	}
}

// streamLatest writes r's Running row (name, count, latest item) as one
// durable line and restarts its idle window: this line proved the Task
// alive, so the heartbeat covers only the silence after it.
func (s *Stream) streamLatest(r *row, now time.Time) {
	r.mark.started, r.mark.lastLine, r.mark.owed = true, now, false
	snap := s.cfg.Source.Snapshot(r.ID)
	snap.Name, snap.State = r.streamName(), record.Running
	var b strings.Builder
	project.WriteTask(&b, snap, s.cfg.Style)
	if b.Len() == 0 {
		return
	}
	s.Write(b.String())
	r.milestone(now)
}
