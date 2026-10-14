package evo

import (
	"slices"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/misuse"
	"github.com/zachbornheimer/evident-output/internal/process"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/live"
	renderplain "github.com/zachbornheimer/evident-output/internal/project/plain"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// Output is the aggregate root for one command's presentation lifecycle.
type Run struct {
	// mu guards the render state. Its sections hold the run's notifications,
	// so the engine's listener hears a section's writes once the mutex is free.
	mu  record.Mutex
	cfg config

	outputID string
	// startedAt is the wire v2 envelope's run_id-adjacent "started_at"
	// (spec §35) — captured once at construction through the Clock facade,
	// read back (never re-derived) at Finish.
	startedAt time.Time
	version   uint64
	closed    bool
	// closing is non-nil once a Close call claimed the teardown; it closes
	// when that teardown ends, so a concurrent Close waits instead of
	// tearing down twice.
	closing chan struct{}
	// confirmTurn lets one Confirm at a time own the terminal.
	confirmTurn *terminal.Turn
	finishing   bool
	finished    bool
	// misuse records the first API misuse of this run. Graph workers call it
	// without Output.mu; a strict recorder panics after recording, never
	// under graph.mu.
	misuse *misuse.Recorder
	// board is the live model of the declared Tasks and collections; live
	// paints it on an interactive surface, nil without one.
	board *live.Board
	live  *live.Engine
	// stream is the run's human durable text: every library-owned line,
	// streamed rows, heartbeats and Finish's residual.
	stream *renderplain.Stream
	// rows are the declared Tasks and collections as every projection
	// shows them; follow keeps the projections up to date with the record;
	// events is the evo.event stream.
	rows       *txt.Rows
	follow     *txt.Follower
	wireEvents *wire.Events

	// changes performs the File, Exec, Patch and Effect operations of this
	// Run, and owns the workspace relative paths resolve against.
	changes *change.Run

	tasks []*taskState
	// rootTasks are the tasks outside every collection, in declaration
	// order: the ones a live frame walks beside the collections.
	rootTasks   []*taskState
	collections []*tasksState
	// rec holds the run's truth: its event journal, ledger, history lines,
	// messages, diagnostics, run-scoped facts and warnings, and Conclusion.
	rec *record.Run

	// graph is the declared shape of the run: identity, keys and numbering.
	graph *graph.Graph
	// taskStates and containerStates are engine's own state for each node the
	// graph declared, keyed by the node's ID.
	taskStates      map[string]*taskState
	containerStates map[string]*tasksState

	// Print/Printf/Println line buffer.
	pendingPrint strings.Builder
	pendingVis   visibility // visibility of the current pending fragment

	// freshness is this Run's lazily opened reconciliation manifest (spec
	// §11.3) and what it remembers about each Task: declared inputs,
	// observed Basis, pending operation records and Verify checks.
	freshness *freshness.Run
	// cleanStops tracks running Tasks that end Succeeded on a named signal.
	cleanStops cleanStopRegistry
}

// baseConfig is the config a run starts from before Config resolves
// into it.
func baseConfig(subject string) config {
	return config{
		subject:         subject,
		clock:           systemClock{},
		visibilityDelay: defaultVisibilityDelay,
		maxFrameRate:    defaultMaxFrameRate,
		width:           defaultWidth,
		debugLevel:      levelInfo,
		redactor:        process.NoopRedactor{},
		maxEntities:     defaultMaxEntities,
		verbosity:       verbosityNormal,
		processRunner:   process.System(),
		fileFS:          fs.Disk{},
	}
}

// newRun builds the run cfg describes: its graph, freshness and changes, and
// every projection of it.
func newRun(cfg config) *Run {
	// A Terminal driver supplied without to() must still land its
	// non-interactive/residual projection somewhere (release-gate round 8
	// finding 2): default primary to the driver's own Sink() when it
	// reports one. A driver that explicitly reports no fixed sink (nil —
	// e.g. a virtual test screen driving output straight through the live
	// surface) is left alone; a driver that cannot answer the question at
	// all is misuse, recorded below once o exists.
	//
	// Whenever the driver's sink IS the primary writer — whether that's this
	// same defaulting, or an explicit to()/withDiagnostics() (Options path) or
	// Config's own to(c.Facades.Stdout)/to(c.Facades.Stderr) wiring (Config.Terminal path,
	// via configToOptions) that happens to coincide — the driver already
	// rendered the conclusion there once; Finish's dual-write branch must
	// not render it there again (release-gate round 9 finding 1).
	terminalWithoutSink := false
	if cfg.terminal != nil {
		sr, ok := cfg.terminal.(sinkReporter)
		switch {
		case !ok:
			terminalWithoutSink = cfg.primary == nil
		case sr.Sink() == nil:
			// Explicit "no fixed sink" — left alone.
		case cfg.primary == nil:
			cfg.primary = sr.Sink()
			cfg.samePrimaryAsTerminal = true
		case sr.Sink() == cfg.primary:
			cfg.samePrimaryAsTerminal = true
		case sr.Sink() == cfg.diagnostic:
			// Driver owns the diagnostic stream; primary is a distinct
			// stream the caller separately configured — both streams get
			// their own copy by design, not a duplicate.
		}
	}
	// An Options build that installs neither to() nor a Terminal sink must
	// still land its residual/conclusion projection somewhere — before this,
	// primary stayed nil and Finish silently wrote zero bytes, even on a
	// Fail (exit 2 with no evidence of why). Default to os.Stdout, matching
	// the non-Options default, and apply the same TTY/color inference the
	// non-Options path applies to that stream: never override an explicit
	// withNoColor(), only ever strengthen it, so a defaulted destination piped
	// to a file never leaks raw ANSI into it (release-gate round 9 findings
	// 2 and 5).
	if cfg.terminal == nil && cfg.primary == nil {
		cfg.primary = terminal.Stdout()
		if !cfg.noColor && (lookupEnv(envKeyNoColor) != "" || !writerIsCharDevice(cfg.primary)) {
			cfg.noColor = true
		}
	}
	if cfg.maxEntities <= 0 {
		cfg.maxEntities = defaultMaxEntities
	}
	if cfg.maxEvents <= 0 {
		cfg.maxEvents = defaultMaxEvents
	}
	resolveGlyphProfileLocked(&cfg)
	run := record.NewRun()
	o := &Run{
		cfg:             cfg,
		rec:             run,
		misuse:          misuse.New(cfg.strict),
		outputID:        "out_1",
		confirmTurn:     terminal.NewTurn(),
		taskStates:      make(map[string]*taskState),
		containerStates: make(map[string]*tasksState),
	}
	host := runHost{out: o}
	o.graph = graph.New(run, graph.WithMaxEntities(cfg.maxEntities), graph.WithMaxConcurrency(cfg.maxConcurrency), graph.WithMisuseSink(o.misuse), graph.WithHost(host))
	o.freshness = freshness.NewRun(o.graph, freshness.RunConfig{Where: o.manifestConfig, Closed: errClosed, DryRun: host.DryRun, Narration: host})
	o.changes = change.NewRun(o.newChangeHost(), o.graph)
	o.rows = txt.NewRows(rowLens{o})
	o.board = live.NewBoard(run, boardSource{o})
	if surface := liveSurfaceOf(cfg); surface != nil {
		o.live = live.NewEngine(live.Config{
			Surface: surface, Board: o.board, Scene: liveScene{o}, Lock: &o.mu, Clock: cfg.clock,
			VisibilityDelay: cfg.visibilityDelay, MaxFrameRate: cfg.maxFrameRate,
			Style: o.humanStyle(), Subject: cfg.subject,
		})
	}
	o.stream = renderplain.NewStream(renderplain.Config{
		Run: run, Region: o.live, Source: o.rows, Sinks: humanSinks{o}, Scene: liveScene{o},
		Lock: &o.mu, Clock: cfg.clock, Style: o.humanStyle(),
		SuppressHuman: cfg.projection.suppressesHuman(), Pane: cfg.debugPanePresentation(),
	})
	o.follow = txt.NewFollower(run, &o.mu, settleReactions{o})
	o.mu.Bind(run)
	run.Listen(o.follow)
	// Stable-enough id for a process-local output instance.
	o.outputID = o.graph.NextID("out")
	o.wireEvents = wire.NewEvents(cfg.eventStream(), o.outputID, cfg.clock)
	o.startedAt = o.cfg.clock.Now()
	o.appendEventLocked(recEvent{Type: "output.started", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventRunStarted, "", runStartedPayload(o.cfg.subject))
	if terminalWithoutSink {
		o.recordMisuse(errTerminalWithoutSink)
	}
	if cfg.dryRun {
		// Announce before any task/item can reach the durable stream: no
		// caller path can finish a DryRun-configured Output without this
		// line having appeared first (evo-rec.md Problem 1). Safe to write
		// unlocked — o has not yet been returned to the caller.
		o.emitPlannedHeaderLocked()
	}
	return o
}

// manifestConfig names the manifest this Run reads: the configured
// application id and state directory, scoped to the Run's workspace.
func (o *Run) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: o.changes.Workspace()}
}

// emitPlannedHeaderLocked writes the planned run's opening announcement
// immediately, once, through the same durable-write path
// (writeDurableTextLocked) every other library-owned line uses.
func (o *Run) emitPlannedHeaderLocked() {
	var b strings.Builder
	renderplain.WritePlannedHeader(&b, !o.cfg.noColor, o.cfg.preview, o.cfg.dryRunHeaderText)
	o.stream.Write(b.String())
}

// promoteRunningLocked transitions a Pending task to Running on its first
// unit of evidence (Phase/Progress/Advance/Bytes/Step/Writer
// write). A work callback starting is announced by taskStarted, since the
// scheduler's claim already moved the row.
// For a sequential collection (Sequence), it records misuse when a sibling is
// already Running, enforcing the heart contract "one Running child"
// (evo-rec.md) — callers still get the transition; Strict mode is what
// escalates the violation to a panic. A plain Group collection
// documents its children as independent (worker-pool fan-out is a
// supported, concurrency-safe pattern there), so it is not policed.
func (o *Run) promoteRunningLocked(st *taskState) {
	st.node.MarkRunning()
	o.announceRunningLocked(st)
}

// announceRunningLocked is the render state a Task owes for becoming Running,
// whoever moved its row: the Sequence check, the filing, the plain heartbeat
// and the task.started event. It answers once per Task.
func (o *Run) announceRunningLocked(st *taskState) {
	if st.runningAnnounced {
		return
	}
	st.runningAnnounced = true
	if row := o.rows.Task(st.node.TaskID()); row != nil {
		row.Timing.Started = o.cfg.clock.Now()
	}
	if col := st.collection(); col != nil && col.node.Sequential {
		col.runningSteps = slices.DeleteFunc(col.runningSteps, func(s *taskState) bool { return s.node.Rec.State() != running })
		if len(col.runningSteps) > 0 {
			o.recordMisuse(errConcurrentRunning)
		}
		col.runningSteps = append(col.runningSteps, st)
	}
	o.board.Changed(st.node.TaskID())
	o.stream.Started(st.node.TaskID(), o.cfg.clock.Now())
	o.emitWireEventLocked(wire.EventTaskStarted, st.id, nil)
}

func (o *Run) bumpLocked() {
	o.version++
}

// appendEventLocked journals e. The journal is in the order things happened,
// so the record is followed first: a settle a graph goroutine made on its own
// is journaled before an event this section writes after it, however late the
// record's listener hears it.
func (o *Run) appendEventLocked(e recEvent) {
	o.followRecordLocked()
	e.Timestamp = o.cfg.clock.Now()
	e.SchemaVersion = eventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = o.outputID
	}
	e = o.graph.Root().Journal(e, o.cfg.maxEvents)
	if o.cfg.projection == projectionStreamJSON {
		wire.WriteJournalLine(o.cfg.primary, e)
	}
}

// settleReactions are what this run's projections owe a change to a
// Task, as the follower (project.Follower) asks for them.
type settleReactions struct{ o *Run }

// Changed follows one write to the Task id on the live board and reports
// whether its settle is owed an answer, once per Task.
func (r settleReactions) Changed(id record.TaskID) (record.State, bool) {
	st := r.o.taskStates[string(id)]
	if st == nil {
		return "", false
	}
	to, followed := r.o.board.Follow(id)
	if !followed {
		to = st.node.Rec.State()
	}
	if !st.node.Rec.IsSettled() || st.settleAnswered {
		return to, false
	}
	st.settleAnswered = true
	return to, true
}

// Settled is the work a settle owes, whichever code settled the Task: its
// timing, the live filing, the plain heartbeat, the snapshot version and
// the task.<state> event.
func (r settleReactions) Settled(id record.TaskID, to record.State) {
	o := r.o
	st := o.taskStates[string(id)]
	if row := o.rows.Task(id); row != nil {
		row.Timing.Settled = o.cfg.clock.Now()
	}
	if o.wireEvents.On() {
		o.wireEvents.Emit(wire.EventTaskFinished, st.id, taskFinishedPayload(st))
	}
	if st.node.IsGate() {
		o.bumpLocked()
		return
	}
	o.board.Changed(id)
	o.stream.Settled(id)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task." + string(to), EntityID: st.id})
}

// followRecordLocked brings every projection up to date with the record.
// Callers hold o.mu.
func (o *Run) followRecordLocked() { o.follow.Follow() }

type taskState struct {
	id string
	// node is the graph's declaration of this Task. It owns the Task's stable
	// key and everything the scheduler knows: its phase, predecessors and
	// the work Define submitted.
	node   *graph.Task
	name   string
	handle *Task

	// settleAnswered is set once the render work a settle owes was queued.
	settleAnswered bool
	// runningAnnounced is set once the render work becoming Running owes was
	// done, so task.started fires once however the row reached Running.
	runningAnnounced bool
}

type tasksState struct {
	id string
	// out is the Output this container belongs to: a read of its live index
	// first follows the record (see Output.followRecordLocked).
	out *Run
	// node is the graph's declaration of this Group/Sequence and owns its
	// §3.1 stable key.
	node   *graph.Container
	name   string
	handle *Group

	// runningSteps holds the children promoteRunningLocked moved to
	// Running that may still be Running (pruned on each promotion), so the
	// "one Running child" check never rescans every step.
	runningSteps []*taskState
}

// A container's members belong to the graph: it declared them, in order, and
// owns who contains whom. The engine keeps only each member's render state,
// in the Output's tables keyed by the member's id, and reads the membership
// through the node.

// taskStates are the render states of the Tasks g declared directly, in
// declaration order.
func (g *tasksState) taskStates() []*taskState {
	nodes := g.node.Tasks()
	states := make([]*taskState, len(nodes))
	for i, node := range nodes {
		states[i] = g.out.taskStates[node.ID]
	}
	return states
}

// collection is the container st was declared under, nil for a root Task.
func (st *taskState) collection() *tasksState {
	if st.node.Parent == nil {
		return nil
	}
	return st.handle.out.containerStates[st.node.Parent.ID]
}

// appendTaskLocked gives st a row in every projection, in declaration
// order: synthetic for the Task the library invented to carry the run's
// own ending. Callers hold o.mu.
func (o *Run) appendTaskLocked(st *taskState, synthetic bool) {
	in, groupName := "", ""
	if col := st.collection(); col != nil {
		in, groupName = col.id, col.name
	}
	id := st.node.TaskID()
	row := txt.DeclaredTask{ID: id, Node: st.node, Name: st.name, Declaration: st.node.Declaration, Rec: st.node.Rec, Synthetic: synthetic}
	if !synthetic {
		row.Timing.Declared = o.cfg.clock.Now()
	}
	o.rows.DeclareTask(row, in)
	o.board.DeclareTask(id, st.name, st.node.Rec, in)
	o.stream.Declare(renderplain.Declared{ID: id, Name: st.name, Rec: st.node.Rec, Group: groupName, InGroup: in != ""})
	o.tasks = append(o.tasks, st)
	if in == "" {
		o.rootTasks = append(o.rootTasks, st)
	}
}

// liveScene is what the live engine reads of this run.
type liveScene struct{ o *Run }

func (s liveScene) Ended() bool { return s.o.closed || s.o.finished }

func (s liveScene) Resized(width int) { s.o.cfg.width = width }

func (s liveScene) Width() int { return s.o.cfg.width }

// Pane is the debug pane (§21.3.2) a frame draws under its rows.
func (s liveScene) Pane(width, height int) (string, int) {
	o := s.o
	recorded := o.rec.DebugRecordCount()
	if o.cfg.debugPresentation != debugPresentationPane || recorded == 0 {
		return "", 0
	}
	var pane strings.Builder
	txt.WriteDebugPane(&pane, o.rec.DebugRecordTail(o.cfg.debugPane.Rows()), o.cfg.debugPane, width, o.humanStyle().Color)
	return pane.String(), min(o.cfg.debugPane.ReservedRows(recorded), height-1)
}

// rowLens is what the row model reads of the live board and of freshness.
type rowLens struct{ o *Run }

func (l rowLens) FirstSeen(id record.TaskID) time.Time { return l.o.board.FirstSeen(id) }

func (l rowLens) Tail(id record.TaskID) record.LiveTail { return l.o.board.Tail(id) }

func (l rowLens) States(group string) txt.StateCounts { return l.o.board.States(group) }

func (l rowLens) Tracked(id record.TaskID) record.TaskOperations {
	return l.o.freshness.Tasks().Tracked(id)
}

// boardSource is what the live board reads of this run's Tasks and
// collections.
type boardSource struct{ o *Run }

func (s boardSource) Pull() { s.o.follow.Pull() }

func (s boardSource) View(id record.TaskID) record.TaskSnapshot { return s.o.rows.View(id) }

func (s boardSource) Snapshot(id record.TaskID) record.TaskSnapshot { return s.o.rows.Snapshot(id) }

func (s boardSource) Header(id string) record.TasksSnapshot { return s.o.rows.Header(id) }

func (s boardSource) Emitted(id record.TaskID) bool {
	return s.o.stream.Emitted(id)
}

func (s boardSource) HeldBack(view *record.TaskSnapshot) bool {
	return s.o.stream.HeldBack(*view)
}

// arm marks the live surface as ready to paint before any entity is declared,
// so Init honors the ≤100ms first-paint contract even when the caller does
// heavy work before the first Task. Idempotent.
func (o *Run) arm() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.board.Arm() {
		o.live.Signal(false)
	}
}

// snapshotLocked is the run as every projection shows it now. Callers
// hold o.mu.
func (o *Run) snapshotLocked() snapshot {
	o.followRecordLocked()
	s := o.rows.Into(snapshot{
		Version:       o.version,
		OutputID:      o.outputID,
		Subject:       o.cfg.subject,
		Lines:         o.rec.Lines(),
		Actions:       o.rows.Actions(),
		Timestamp:     o.cfg.clock.Now(),
		DryRun:        o.cfg.dryRun,
		Preview:       o.cfg.preview,
		DryRunSubject: o.cfg.dryRunHeaderText,
		Warnings:      o.rec.RunWarnings(),
		Facts:         o.rec.RunFacts(),
	})
	s.Changes = o.rec.ChangeSnapshots()
	s.Plans = o.rec.PlanSnapshots()
	s.Messages = o.rec.Messages()
	if c, ok := o.rec.Conclusion(); ok {
		s.Conclusion = &c
	}
	return s
}

// emitWireEventLocked writes one §38 "evo.event" line for eventType about
// entityID ("" for the run); a run that streams no events writes nothing.
func (o *Run) emitWireEventLocked(eventType, entityID string, payload map[string]any) {
	o.wireEvents.Emit(eventType, entityID, payload)
}

// emitCollectionDeclaredLocked emits collection.declared (§38) for a newly
// declared Group or Sequence, at the root or nested.
func (o *Run) emitCollectionDeclaredLocked(st *tasksState, parentID string) {
	kind := wire.CollectionKindGroup
	if st.node.Sequential {
		kind = wire.CollectionKindSequence
	}
	o.emitWireEventLocked(wire.EventCollectionDeclared, st.id, map[string]any{
		"name":      st.name,
		"kind":      kind,
		"parent_id": parentID,
	})
}

// Output selects the presentation encoding (CONTRACT §6).
type Output int

const (
	Human Output = iota + 1
	Plain
	JSON
	JSONL
)

// runStartedPayload carries the run's title on run.started; an untitled run
// has no payload.
func runStartedPayload(title string) map[string]any {
	if title == "" {
		return nil
	}
	return map[string]any{"name": title}
}
