package plain

import (
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Stream is a run's human durable text: every line the library writes goes
// through it, in the order things happened (§17.5: a resolved row renders
// at once, not at Finish). It decides what streams while the run works —
// resolved root rows, settled collections as whole blocks, named ledger
// sections, a Running Task's milestones, heartbeats, debug history — and what
// Finish still owes (Residual).
//
// A Stream is not safe for concurrent use: the run's lock guards it. Only
// its heartbeat callbacks take that lock themselves.
type Stream struct {
	cfg Config

	rows  map[record.TaskID]*row
	roots []*row
	// declared counts every Task with a row.
	declared int
	column   rootColumn

	// linesEmitted is how many of the record's lines already reached the
	// human stream; written counts every durable row, streamed or held.
	linesEmitted int
	written      int

	// paneActive is set once a debug record was presented by the live pane
	// rather than as history: it decides the failure tail (§21.3.2).
	paneActive bool

	// streamed are the ledger sections, by id, whose named rows already
	// streamed at their Task's resolution.
	streamed map[string]bool

	// blocks are the collections by id; waiting the root blocks that wait
	// for their membership to close before they stream (see block.go).
	blocks  map[string]*block
	waiting []*block
}

// Config is what a Stream writes and how.
type Config struct {
	Run    *record.Run
	Region Region
	Source Source
	Sinks  Sinks
	Scene  Scene
	// Lock is the run's lock; heartbeat callbacks take it.
	Lock sync.Locker
	// Clock is the domain clock. Heartbeats arm only when it is also a
	// clock.Scheduler.
	Clock clock.Clock
	Style project.Style
	// Pane is the debug pane when debug records present in the live
	// region, nil when they stream as history.
	Pane *project.DebugPane
}

// Region is the live region human text clears and redraws around. A run
// with no live surface passes a Region that is never interactive.
type Region interface {
	Interactive() bool
	// Hold keeps text while a window owns the terminal, and reports
	// whether it did.
	Hold(text string) bool
	// Flush hands every held text to emit.
	Flush(emit func(text string))
	// Durable writes text above the region, and reports whether the
	// surface took it.
	Durable(text string) bool
	// Refresh brings the region in line after a row left it for
	// scrollback.
	Refresh()
	Signal(force bool)
}

// Source is what a Stream reads of a Task beyond its record.
type Source interface {
	Snapshot(id record.TaskID) record.TaskSnapshot
	// Closed reports that nothing more is expected to be declared into the
	// collection id.
	Closed(id string) bool
	// RootLevel is the run's root level as the human projection reads it,
	// the collection detailed in full.
	RootLevel(detailed string) record.Snapshot
}

// Sinks are the writers human text reaches, as the run wires them.
type Sinks interface {
	// Human writes text to the primary stream and every mirror.
	Human(text string)
	// Diagnostic writes text to the diagnostic stream, and reports whether
	// there is one.
	Diagnostic(text string) bool
	// Separate reports a diagnostic stream distinct from the primary one;
	// SharesTerminal that both reach the one terminal a live region owns.
	Separate() bool
	SharesTerminal() bool
}

// Scene is what a Stream reads of the run around it.
type Scene interface {
	// Ended reports the run closed or finished: no heartbeat writes again.
	Ended() bool
	// Width is the terminal width the ledger lays out for.
	Width() int
}

// Declared is one Task a Stream keeps a row for.
type Declared struct {
	ID   record.TaskID
	Name string
	Rec  *record.Task
	// Group is the name and GroupID the id of the collection the Task sits
	// in, when InGroup.
	Group   string
	GroupID string
	InGroup bool
}

// row is one Task as the durable stream keeps it.
type row struct {
	Declared
	// emitted is set once the row reached scrollback, and emittedAs is the
	// state it showed there: a Task that settles again in another state
	// (Done, then overturned to Failed) is written again.
	emitted   bool
	emittedAs record.State
	// ownsWindow is set on a Confirm gate while its window is open: the
	// window shows this row, so it is not held back like a sibling's.
	ownsWindow bool
	mark       mark
	beat       beat
	// block is the root collection the Task sits under, nil at the root;
	// counted is set once its settling was taken off the block's open count.
	block   *block
	counted bool
}

// NewStream starts a run's durable stream.
func NewStream(cfg Config) *Stream {
	return &Stream{
		cfg: cfg, rows: make(map[record.TaskID]*row), streamed: make(map[string]bool),
		blocks: make(map[string]*block),
	}
}

// Declare keeps a row for t.
func (s *Stream) Declare(t Declared) {
	r := &row{Declared: t}
	s.rows[t.ID] = r
	s.declared++
	if !t.InGroup {
		s.roots = append(s.roots, r)
		s.column.add(t.Name)
		return
	}
	s.enter(r)
}

// Write emits durable text now, unless a window owns the terminal: then it
// is held and prints in arrival order once the last window closes, so a
// sibling's row never lands inside a Confirm prompt.
func (s *Stream) Write(text string) {
	if !s.count(text) || s.cfg.Region.Hold(text) {
		return
	}
	s.Emit(text)
}

// WritePayload emits data-payload text that shares the live region's terminal: held while a window owns
// the terminal, else written above the region. It is not a durable row. It reports false when the live
// surface did not take the text, and the caller writes it to its own stream.
func (s *Stream) WritePayload(text string) bool {
	if text == "" || s.cfg.Region.Hold(text) {
		return true
	}
	return s.cfg.Region.Durable(text)
}

// WriteOwner emits text the owner of an open window writes inside it
// (Confirm's prompt and its answer row): the window exists to show exactly
// this, so it is never held.
func (s *Stream) WriteOwner(text string) {
	if s.count(text) {
		s.Emit(text)
	}
}

// count counts text as a durable row and reports whether it is one.
func (s *Stream) count(text string) bool {
	if text == "" {
		return false
	}
	s.written++
	return true
}

// Emit writes text to the human stream now: above the live region when one
// is on the terminal, else to the primary stream and its mirrors.
func (s *Stream) Emit(text string) {
	if s.cfg.Region.Durable(text) {
		return
	}
	s.cfg.Sinks.Human(text)
}

// Flush prints the text held while windows were open.
func (s *Stream) Flush() { s.cfg.Region.Flush(s.Emit) }

// Written is how many durable rows the run has written, held ones included.
func (s *Stream) Written() int { return s.written }

// Lines streams the record's lines no earlier call streamed.
func (s *Stream) Lines() {
	pending := s.cfg.Run.LinesFrom(s.linesEmitted)
	if len(pending) == 0 {
		return
	}
	var b strings.Builder
	for _, line := range pending {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	s.linesEmitted = s.cfg.Run.LineCount()
	s.Write(b.String())
}

// SkipLines marks every line the record holds as already shown elsewhere.
func (s *Stream) SkipLines() { s.linesEmitted = s.cfg.Run.LineCount() }

// Emitted reports whether the Task id's row is already in scrollback.
func (s *Stream) Emitted(id record.TaskID) bool {
	r := s.rows[id]
	return r != nil && r.emitted
}

// OwnWindow marks the Task id as the one its open window shows, or clears
// the mark.
func (s *Stream) OwnWindow(id record.TaskID, owns bool) {
	if r := s.rows[id]; r != nil {
		r.ownsWindow = owns
	}
}
