package plain

import (
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// heartbeatInterval is how often silent long-running work earns an
// automatic durable heartbeat line in plain human output (spec §40):
// "• generate schema  — 30s", then "— 1m", and so on — proof of life for a
// CI log between real milestones. Far slower than the live terminal's 100ms
// rule (§23.1): the live region proves liveness with spinner motion.
const heartbeatInterval = 30 * time.Second

// beat is one Task's heartbeat.
type beat struct {
	// runningAt is the elapsed anchor: when the Task became Running, or its
	// latest real milestone. Zero means no heartbeat is armed (a live
	// terminal, or a clock that cannot schedule).
	runningAt time.Time
	// due is when the next check should write a line, pushed out by every
	// real milestone so narrating work never also gets a heartbeat.
	due time.Time
	// stop cancels the pending check, so a settled Task's timer neither
	// fires nor keeps the run reachable until it would have.
	stop func()
}

// scheduler is the clock that can arm heartbeats: plain presentation only
// (the live terminal owns its own liveness) on a clock that schedules.
func (s *Stream) scheduler() (clock.Scheduler, bool) {
	sched, ok := s.cfg.Clock.(clock.Scheduler)
	if !ok || s.cfg.Region.Interactive() {
		return nil, false
	}
	return sched, true
}

// Started arms the Task id's heartbeat: its first check is due one interval
// after now.
func (s *Stream) Started(id record.TaskID, now time.Time) {
	r := s.rows[id]
	sched, ok := s.scheduler()
	if r == nil || !ok {
		return
	}
	r.beat.runningAt = now
	r.beat.due = now.Add(heartbeatInterval)
	s.schedule(sched, r, heartbeatInterval)
}

// milestone restarts r's idle window from a real durable emission: "stops
// on any real milestone" (§40) supersedes the pending heartbeat, it does
// not revoke heartbeats, so a Task that goes silent again earns its next
// one an interval after this update, showing elapsed time from it.
func (r *row) milestone(now time.Time) {
	if r.beat.runningAt.IsZero() {
		return
	}
	r.beat.runningAt = now
	r.beat.due = now.Add(heartbeatInterval)
}

// schedule arms one check of r, due after wait.
func (s *Stream) schedule(sched clock.Scheduler, r *row, wait time.Duration) {
	if wait <= 0 {
		wait = heartbeatInterval
	}
	id := r.ID
	r.beat.stop = sched.AfterFunc(wait, func() { s.check(id) })
}

// Settled stops the Task id's heartbeat: a settled Task never earns another.
func (s *Stream) Settled(id record.TaskID) {
	if r := s.rows[id]; r != nil {
		r.stopBeat()
	}
}

// StopHeartbeats cancels every pending heartbeat, so a dropped run is
// unreachable even when Finish already settled (or never did).
func (s *Stream) StopHeartbeats() {
	for _, r := range s.rows {
		r.stopBeat()
	}
}

func (r *row) stopBeat() {
	if r.beat.stop == nil {
		return
	}
	r.beat.stop()
	r.beat.stop = nil
}

// check is a scheduled heartbeat check. It takes the run's lock (the
// scheduler calls it on a timer goroutine) and either does nothing for a
// Task that settled or a run that ended, re-arms for the remaining wait when
// a milestone moved the due time, or writes one heartbeat and re-arms.
func (s *Stream) check(id record.TaskID) {
	s.cfg.Lock.Lock()
	defer s.cfg.Lock.Unlock()
	if s.cfg.Scene.Ended() {
		return
	}
	r := s.rows[id]
	if r == nil || record.IsTerminalTask(r.Rec.State()) || r.beat.runningAt.IsZero() {
		return
	}
	sched, ok := s.scheduler()
	if !ok {
		return
	}
	now := s.cfg.Clock.Now()
	if now.Before(r.beat.due) {
		s.schedule(sched, r, r.beat.due.Sub(now))
		return
	}
	s.heartbeat(r, now)
	r.beat.due = now.Add(heartbeatInterval)
	s.schedule(sched, r, heartbeatInterval)
}

// heartbeat writes one "• <name>  — <elapsed>" line in the row grammar
// every plain row shares, with its own glyph so it never reads as a real
// update.
func (s *Stream) heartbeat(r *row, now time.Time) {
	st := s.cfg.Style
	unit := project.DisplayUnit{
		Glyph:  project.StyleGlyph(project.GlyphHeartbeat.Render(st.Profile), project.StateColor(record.Running), st.Color),
		Name:   r.streamName(),
		Detail: project.Dim("— "+project.FormatElapsed(now.Sub(r.beat.runningAt)), st.Color),
	}
	s.Write(unit.Render("") + "\n")
}
