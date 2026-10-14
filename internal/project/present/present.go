// Package present shows one run: it composes the shared row model, the live
// board and region, the durable stream and the evo.event stream, and keeps
// them current with the record through one follower. The root package holds
// one Presenter; graph, freshness and change never see it.
package present

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/zachbornheimer/evident-output/internal/clock"
	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/live"
	"github.com/zachbornheimer/evident-output/internal/project/plain"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Presenter is how one run is shown. Its exported parts are the
// projections the root drives directly; Presenter keeps them consistent.
//
// A Presenter is not safe for concurrent use: the run's lock guards it.
type Presenter struct {
	Rows   *project.Rows
	Board  *live.Board
	Live   *live.Engine // nil without a live surface; its methods are nil-safe
	Stream *plain.Stream
	Events *wire.Events

	cfg    Config
	follow *project.Follower
	tasks  map[record.TaskID]*task
	// version counts the changes a snapshot reports.
	version uint64
}

// task is the presentation bookkeeping of one Task, a gate included.
type task struct {
	rec  *record.Task
	gate bool
	// started is set once becoming Running was answered, however the row got
	// there. answered is how many of the record's settlements were answered.
	started  bool
	answered int
}

// Config is what a Presenter shows and where.
type Config struct {
	Run   *record.Run
	Lock  sync.Locker
	Clock clock.Clock
	Host  Host
	// RunID names the run on the evo.event stream; Events receives that
	// stream, nil for none.
	RunID  string
	Events io.Writer
	// Surface is the interactive terminal a live region paints on, nil for
	// none.
	Surface         live.Surface
	VisibilityDelay time.Duration
	MaxFrameRate    int
	Style           project.Style
	Subject         string
	Sinks           plain.Sinks
	Pane            *project.DebugPane
}

// Host is what a Presenter reads of the run around it.
type Host interface {
	// Ended reports the run closed or finished.
	Ended() bool
	// Width is the terminal width the ledger lays out for; Resize records
	// a new one the live surface reported.
	Width() int
	Resize(width int)
	// Tracked is what the Task id's File and Exec operations observed.
	Tracked(id record.TaskID) record.TaskOperations
	// Journal records e in the run's event journal.
	Journal(e record.Event)
}

// New shows the run cfg names. It starts following the record at once.
func New(cfg Config) *Presenter {
	p := &Presenter{cfg: cfg, tasks: make(map[record.TaskID]*task)}
	p.Rows = project.NewRows(lens{p})
	p.Board = live.NewBoard(cfg.Run, source{p})
	if cfg.Surface != nil {
		p.Live = live.NewEngine(live.Config{
			Surface: cfg.Surface, Board: p.Board, Scene: scene{p}, Lock: cfg.Lock, Clock: cfg.Clock,
			VisibilityDelay: cfg.VisibilityDelay, MaxFrameRate: cfg.MaxFrameRate,
			Style: cfg.Style, Subject: cfg.Subject,
		})
	}
	p.Stream = plain.NewStream(plain.Config{
		Run: cfg.Run, Region: p.Live, Source: p.Rows, Sinks: cfg.Sinks, Scene: scene{p},
		Lock: cfg.Lock, Clock: cfg.Clock, Style: cfg.Style,
		Pane: cfg.Pane,
	})
	p.Events = wire.NewEvents(cfg.Events, cfg.RunID, cfg.Clock)
	p.follow = project.NewFollower(cfg.Run, cfg.Lock, reactions{p})
	cfg.Run.Listen(p)
	return p
}

// TaskChanged and TaskSettled make a Presenter the run's record.Listener:
// it hears the record through its follower.
func (p *Presenter) TaskChanged(id record.TaskID, from, to record.State) {
	p.follow.TaskChanged(id, from, to)
}

func (p *Presenter) TaskSettled(id record.TaskID, from, to record.State) {
	p.follow.TaskSettled(id, from, to)
}

// Version is how many changes a snapshot has seen; Bump counts one more.
func (p *Presenter) Version() uint64 { return p.version }

// Bump counts one change a snapshot reports.
func (p *Presenter) Bump() { p.version++ }

// Follow brings every projection up to date with the record.
func (p *Presenter) Follow() { p.follow.Follow() }

// Narrate writes one evo.event about entityID ("" for the run), building
// payload only when the run streams events.
func (p *Presenter) Narrate(eventType, entityID string, payload func() map[string]any) {
	if p.Events.On() {
		p.Events.Emit(eventType, entityID, payload())
	}
}

// DeclareTask gives t a row in every projection, under the collection in
// or at the root when in is "", and announces it.
func (p *Presenter) DeclareTask(t project.DeclaredTask, in string) {
	p.tasks[t.ID] = &task{rec: t.Rec}
	group := p.Rows.Group(in)
	p.Rows.DeclareTask(t, in)
	p.Board.DeclareTask(t.ID, t.Name, t.Rec, in)
	declared := plain.Declared{ID: t.ID, Name: t.Name, Rec: t.Rec}
	if group != nil {
		declared.Group, declared.InGroup = group.Name, true
	}
	p.Stream.Declare(declared)
	p.version++
	if t.Synthetic {
		p.journal("output."+string(t.Rec.State()), "")
		return
	}
	p.journal("task.declared", string(t.ID))
	p.Narrate(wire.EventTaskDeclared, string(t.ID), func() map[string]any { return map[string]any{"name": t.Name} })
	p.Live.Signal(true)
}

// DeclareGate follows a container builder's gate: no row, in no
// collection, but its settle is answered like any Task's.
func (p *Presenter) DeclareGate(id record.TaskID, rec *record.Task) {
	p.tasks[id] = &task{rec: rec, gate: true}
}

// DeclareGroup gives g a row under the collection parent, or at the root
// when parent is "", and announces it.
func (p *Presenter) DeclareGroup(g project.DeclaredGroup, parent string) {
	p.Rows.DeclareGroup(g, parent)
	p.Board.DeclareGroup(g.ID, g.Name, parent)
	p.version++
	p.journal("tasks.declared", g.ID)
	kind := wire.CollectionKindGroup
	if g.Sequential {
		kind = wire.CollectionKindSequence
	}
	p.Narrate(wire.EventCollectionDeclared, g.ID, func() map[string]any {
		return map[string]any{"name": g.Name, "kind": kind, "parent_id": parent}
	})
}

// Started answers the Task id becoming Running, once however its row got
// there: its timing, its live filing, its heartbeat and task.started. It
// reports whether this call answered it.
func (p *Presenter) Started(id record.TaskID) bool {
	t := p.tasks[id]
	if t == nil || t.started {
		return false
	}
	t.started = true
	now := p.cfg.Clock.Now()
	if row := p.Rows.Task(id); row != nil {
		row.Timing.Started = now
	}
	p.Board.Changed(id)
	p.Stream.Started(id, now)
	p.Narrate(wire.EventTaskStarted, string(id), noPayload)
	return true
}

// Snapshot is the run as every projection shows it now, meta carrying the
// run-scoped fields the caller owns.
func (p *Presenter) Snapshot(meta record.Snapshot) record.Snapshot {
	p.Follow()
	meta.Version = p.version
	meta.Actions = p.Rows.Actions()
	s := p.Rows.Into(meta)
	s.Changes = p.cfg.Run.ChangeSnapshots()
	s.Plans = p.cfg.Run.PlanSnapshots()
	s.Messages = p.cfg.Run.Messages()
	if c, ok := p.cfg.Run.Conclusion(); ok {
		s.Conclusion = &c
	}
	return s
}

// PlannedHeader writes a planned run's opening announcement, once, first:
// no path finishes a dry run without it (evo-rec.md Problem 1).
func (p *Presenter) PlannedHeader(preview bool, subject string) {
	var b strings.Builder
	plain.WritePlannedHeader(&b, p.cfg.Style.Color, preview, subject)
	p.Stream.Write(b.String())
}

// Arm promises a paint before any entity exists, so a run honors the first
// paint rule even when the caller works before its first Task.
func (p *Presenter) Arm() {
	if p.Board.Arm() {
		p.Live.Signal(false)
	}
}

// journal records one event of eventType about entityID.
func (p *Presenter) journal(eventType, entityID string) {
	p.cfg.Host.Journal(record.Event{Type: eventType, EntityID: entityID})
}

func noPayload() map[string]any { return nil }
