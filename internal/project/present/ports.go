package present

import (
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// source is what the live board reads of the run's Tasks and collections.
type source struct{ p *Presenter }

func (s source) Pull()                                         { s.p.follow.Pull() }
func (s source) View(id record.TaskID) record.TaskSnapshot     { return s.p.Rows.View(id) }
func (s source) Snapshot(id record.TaskID) record.TaskSnapshot { return s.p.Rows.Snapshot(id) }
func (s source) Header(id string) record.TasksSnapshot         { return s.p.Rows.Header(id) }
func (s source) Emitted(id record.TaskID) bool                 { return s.p.Stream.Emitted(id) }
func (s source) HeldBack(view *record.TaskSnapshot) bool       { return s.p.Stream.HeldBack(*view) }

// scene is what the live engine and the durable stream read of the run.
type scene struct{ p *Presenter }

func (s scene) Ended() bool       { return s.p.cfg.Host.Ended() }
func (s scene) Width() int        { return s.p.cfg.Host.Width() }
func (s scene) Resized(width int) { s.p.cfg.Host.Resize(width) }

// Pane is the debug pane (§21.3.2) a frame draws under its rows.
func (s scene) Pane(width, height int) (string, int) {
	pane, run := s.p.cfg.Pane, s.p.cfg.Run
	recorded := run.DebugRecordCount()
	if pane == nil || recorded == 0 {
		return "", 0
	}
	var b strings.Builder
	project.WriteDebugPane(&b, run.DebugRecordTail(pane.Rows()), *pane, width, s.p.cfg.Style.Color)
	return b.String(), min(pane.ReservedRows(recorded), height-1)
}

// lens is what the row model reads of the live board and of freshness.
type lens struct{ p *Presenter }

func (l lens) FirstSeen(id record.TaskID) time.Time  { return l.p.Board.FirstSeen(id) }
func (l lens) Tail(id record.TaskID) record.LiveTail { return l.p.Board.Tail(id) }
func (l lens) States(group string) project.StateCounts {
	return l.p.Board.States(group)
}
func (l lens) Tracked(id record.TaskID) record.TaskOperations { return l.p.cfg.Host.Tracked(id) }

// reactions are what the projections owe a change to a Task, as the
// follower asks for them.
type reactions struct{ p *Presenter }

// Changed follows one write to the Task id on the live board and reports
// whether its settle is owed an answer, once per settlement.
func (r reactions) Changed(id record.TaskID) (record.State, bool) {
	t := r.p.tasks[id]
	if t == nil {
		return "", false
	}
	to, followed := r.p.Board.Follow(id)
	if !followed {
		to = t.rec.State()
	}
	settlements := t.rec.TimesSettled()
	if settlements <= t.answered {
		return to, false
	}
	t.answered = settlements
	return to, true
}

// Settled is the work a settle owes, whichever code settled the Task: for a
// builder's gate only a new version, since a gate is no declared Task and
// never reaches evo.event; for a Task, its timing, task.finished, its live
// filing, its heartbeat and the task.<state> journal event.
func (r reactions) Settled(id record.TaskID, to record.State) {
	p, t := r.p, r.p.tasks[id]
	p.version++
	if t.gate {
		return
	}
	if row := p.Rows.Task(id); row != nil {
		row.Timing.Settled = p.cfg.Clock.Now()
	}
	p.Narrate(wire.EventTaskFinished, string(id), func() map[string]any { return finishedPayload(t.rec) })
	p.Board.Changed(id)
	p.Stream.Settled(id)
	p.journal("task."+string(to), string(id))
}

// finishedPayload is task.finished's payload: the Outcome the Task settled
// with, absent for a NotStarted Task exactly as in evo.run, and its Summary
// when it has one (ZYS-971).
func finishedPayload(rec *record.Task) map[string]any {
	payload := map[string]any{}
	if outcome := rec.State().Outcome(); outcome != "" {
		payload["outcome"] = string(outcome)
	}
	if summary := rec.Summary(); summary != "" {
		payload["summary"] = summary
	}
	return payload
}
