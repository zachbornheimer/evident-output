package evo

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"sync/atomic"
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
	// pulled and settles are followRecordLocked's scratch: the Tasks the
	// record reported changed, and the ones among them that settled.
	pulled  []record.TaskID
	settles []settleReaction
	// answeredSettles counts the settles reactToSettlesLocked answered. It
	// is atomic so the listener can see, without o.mu, that a settle it
	// hears was already answered (see outputListener.TaskSettled).
	answeredSettles atomic.Uint64
	// following is set while followRecordLocked runs, so an event it appends
	// does not follow the record again.
	following bool
	closed    bool
	// closing is non-nil once a Close call claimed the teardown; it closes
	// when that teardown ends, so a concurrent Close waits instead of
	// tearing down twice.
	closing chan struct{}
	// confirmTurn holds one token while a Confirm owns the terminal (see
	// awaitConfirmTurn); nil until the first Confirm asks.
	confirmTurn chan struct{}
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

	// wireEventErr latches the first §38 "evo.event" JSONL write failure
	// (structured_events.go's emitWireEventLocked).
	wireEventErr error
	// wireSeq is the last seq the evo.event stream numbered: the stream's own
	// counter, so its lines run 1, 2, 3 ... with no gap a consumer could
	// mistake for a lost line.
	wireSeq uint64

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

func newOutput(subject string, options ...option) *Run {
	cfg := config{
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
	for _, opt := range options {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
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
		taskStates:      make(map[string]*taskState),
		containerStates: make(map[string]*tasksState),
	}
	host := runHost{out: o}
	o.graph = graph.New(run, graph.WithMaxEntities(cfg.maxEntities), graph.WithMaxConcurrency(cfg.maxConcurrency), graph.WithMisuseSink(o.misuse), graph.WithHost(host))
	o.freshness = freshness.NewRun(o.graph, freshness.RunConfig{Where: o.manifestConfig, Closed: errClosed, DryRun: host.DryRun, Narration: host})
	o.changes = change.NewRun(o.newChangeHost(), o.graph)
	o.board = live.NewBoard(run, boardSource{o})
	if surface := liveSurfaceOf(cfg); surface != nil {
		o.live = live.NewEngine(live.Config{
			Surface: surface, Board: o.board, Scene: liveScene{o}, Lock: &o.mu, Clock: cfg.clock,
			VisibilityDelay: cfg.visibilityDelay, MaxFrameRate: cfg.maxFrameRate,
			Style: o.humanStyle(), Subject: cfg.subject,
		})
	}
	o.stream = renderplain.NewStream(renderplain.Config{
		Run: run, Region: o.live, Source: boardSource{o}, Sinks: humanSinks{o}, Scene: liveScene{o},
		Lock: &o.mu, Clock: cfg.clock, Style: o.humanStyle(),
		SuppressHuman: cfg.projection.suppressesHuman(), Pane: cfg.debugPanePresentation(),
	})
	o.mu.Bind(run)
	run.SetListener(outputListener{o: o})
	// Stable-enough id for a process-local output instance.
	o.outputID = o.graph.NextID("out")
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
	st.timing.Started = o.cfg.clock.Now()
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

// --- folded from format.go ---

func init() {
	// Single source of truth: PublishedRelease, pinned once into the
	// v2 wire encoder path (spec §32.1/§35's evo_version).
	setWireEvoVersion(publishedRelease)
}

// PublishedRelease is the single source of truth for the current published
// module and MCP pin used in install guidance.
//
// When cutting a release:
//  1. Promote CHANGELOG ## Unreleased → ## [X.Y.Z] (Keep a Changelog).
//  2. Set publishedRelease to the new tag (e.g. "v1.0.1").
//  3. Prefer: mise run test && mise run cut-release
//  4. Tag that commit; do not move prior tags.
//
// version_drift_test.go enforces the portable surface stays synchronized.
const publishedRelease = "v2.0.0"

// --- folded from journal.go ---

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
		o.writeStreamJSONLocked(e)
	}
}

func (o *Run) writeStreamJSONLocked(e recEvent) {
	w := o.cfg.primary
	if w == nil {
		return
	}
	row, err := wire.EncodeEventJSON(e)
	if err != nil {
		return
	}
	_, _ = w.Write(row)
	_, _ = w.Write([]byte{'\n'})
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
}

// --- folded from listener.go ---

// The engine's render state follows the record by pulling, never by being
// pushed. A write to the record logs its Task while the record's lock is
// held (record.Run.TakeChanged), so anything that reads the live index first
// pulls that log and finds every write the record has seen, whichever
// goroutine made it and whether or not the record has told its listener yet.
// The listener is only a prompt to pull: the engine hears a change made
// outside Output.mu, takes the mutex itself, and follows the record. No
// handler needs to run inside a writer's lock.

// outputListener is how the engine hears a change made by any writer.
type outputListener struct{ o *Run }

// TaskChanged has nothing to do: a change waits in the log for the next
// read of the index.
func (outputListener) TaskChanged(record.TaskID, record.State, record.State) {}

// TaskSettled follows the record, so the settle is answered promptly even
// when nothing reads the index. The record names the settle itself, so the
// engine cannot disagree with the writer about which states end a Task.
//
// Most settles happen inside a section of o.mu that already followed the
// record, so the settle the listener hears is answered by then: the counts
// prove it without taking the mutex. The record's count is read first, so
// an answer that catches up to it covers every settle written before.
func (l outputListener) TaskSettled(record.TaskID, record.State, record.State) {
	if written := l.o.rec.SettledTasks(); l.o.answeredSettles.Load() >= written {
		return
	}
	l.o.mu.Lock()
	defer l.o.mu.Unlock()
	l.o.followRecordLocked()
}

// settleReaction is a Task that settled and the state it settled into.
type settleReaction struct {
	st *taskState
	to entityState
}

// followRecordLocked brings the render state up to date with the record:
// the live index first, then what each terminal transition owes. A call made while
// it is already running does nothing. Callers hold o.mu.
func (o *Run) followRecordLocked() {
	if o.following {
		return
	}
	o.following = true
	defer func() { o.following = false }()
	o.pullRecordLocked()
	o.reactToSettlesLocked()
}

// pullRecordLocked brings the live index (census and filing) up to date with
// every write the record has seen, and notes the Tasks that settled so
// reactToSettlesLocked can answer each once. It changes nothing else, so a
// read of the index may pull in the middle of building a frame. Callers hold
// o.mu.
func (o *Run) pullRecordLocked() {
	o.pulled = o.rec.TakeChanged(o.pulled[:0])
	for _, id := range o.pulled {
		st := o.taskStates[string(id)]
		if st == nil {
			continue
		}
		to, followed := o.board.Follow(id)
		if !followed {
			to = st.node.Rec.State()
		}
		if st.node.Rec.IsSettled() && !st.settleAnswered {
			st.settleAnswered = true
			o.settles = append(o.settles, settleReaction{st: st, to: to})
		}
	}
}

// reactToSettlesLocked answers each Task pullRecordLocked saw settle. Callers
// hold o.mu.
func (o *Run) reactToSettlesLocked() {
	for i := range o.settles {
		o.taskSettledLocked(o.settles[i].st, o.settles[i].to)
		o.settles[i] = settleReaction{}
		o.answeredSettles.Add(1)
	}
	o.settles = o.settles[:0]
}

// taskSettledLocked is the render state a terminal transition owes, whichever
// code settled the Task: the live filing, the plain heartbeat, the snapshot
// version and the task.<state> event. The live census moved when the record
// was pulled. Callers hold o.mu.
func (o *Run) taskSettledLocked(st *taskState, to entityState) {
	st.timing.Settled = o.cfg.clock.Now()
	if o.narratesEvents() {
		o.emitWireEventLocked(wire.EventTaskFinished, st.id, taskFinishedPayload(st))
	}
	if st.node.IsGate() {
		o.bumpLocked()
		return
	}
	o.board.Changed(st.node.TaskID())
	o.stream.Settled(st.node.TaskID())
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task." + string(to), EntityID: st.id})
}

type taskState struct {
	id string
	// node is the graph's declaration of this Task. It owns the Task's stable
	// key and everything the scheduler knows: its phase, predecessors and
	// the work Define submitted.
	node   *graph.Task
	name   string
	handle *Task
	// board holds the Task's live row: its first paint and live tail.
	board *live.Board

	// settleAnswered is set once the render work a settle owes was queued.
	settleAnswered bool
	// runningAnnounced is set once the render work becoming Running owes was
	// done, so task.started fires once however the row reached Running.
	runningAnnounced bool
	// timing is when the Task was declared, began running and settled.
	timing record.TaskTiming

	// synthetic marks a task the library invented to carry an output-level
	// outcome (Output.Fail/Cancel's synthetic "command" task) rather than
	// one the caller declared. shouldSuppressRepeatedCondition (I2) must
	// never drop the standalone conclusion band for one of these: it is the
	// only place the run's outcome is ever stated, unlike a caller-declared
	// Task whose own row already says the same thing.
	synthetic bool
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

// taskCount is how many Tasks g declared directly.
func (g *tasksState) taskCount() int { return g.node.TaskCount() }

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

// childStates are the render states of the Groups and Sequences g declared
// directly, in declaration order.
func (g *tasksState) childStates() []*tasksState {
	nodes := g.node.Children()
	states := make([]*tasksState, len(nodes))
	for i, node := range nodes {
		states[i] = g.out.containerStates[node.ID]
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

// appendTaskLocked adds st to the run's Tasks in declaration order. Caller
// must hold o.mu.
func (o *Run) appendTaskLocked(st *taskState) {
	in, groupName := "", ""
	if col := st.collection(); col != nil {
		in, groupName = col.id, col.name
	}
	o.board.DeclareTask(st.node.TaskID(), st.name, st.node.Rec, in)
	o.stream.Declare(renderplain.Declared{ID: st.node.TaskID(), Name: st.name, Rec: st.node.Rec, Group: groupName, InGroup: in != ""})
	o.tasks = append(o.tasks, st)
	if in == "" {
		o.rootTasks = append(o.rootTasks, st)
	}
}

// rowOf is the Task id that has a row: a declared Task, or the synthetic
// Task carrying the run's ending, which the graph never declared and so
// has no entry in taskStates.
func (o *Run) rowOf(id record.TaskID) *taskState {
	if st := o.taskStates[string(id)]; st != nil {
		return st
	}
	for _, st := range slices.Backward(o.rootTasks) {
		if st.id == string(id) {
			return st
		}
	}
	return nil
}

// Writer returns a line-buffered io.Writer for narrating a talkative child
// process: each complete line (CR or LF terminated, trimmed, non-empty)
// becomes the task's live doing-text (see Doing), and every byte is also
// retained in the task's Capture ring (get-or-create, shared with
// Task.Capture) so DetailTail has proof after Fail. Lines pass through the
// same sanitize layer as Task.Doing, so hostile escape sequences never reach
// the display. Off a TTY, these mirrored lines update the live status only —
// they never force their own durable row the way an explicit
// Task.Doing call does, since the Capture ring (and its failure-path
// DetailTail) is already the child's one durable home (release-gate round 9
// finding 4). Concurrent-safe. Named Writer, not PhaseWriter (P6/rename):
// an io.Writer sink whose lines become the live-status text, following
// logrus's Logger.Writer()/zapio.Writer precedent.
//
//	cmd.Stdout = evo.Task("push").Writer()
func (t *Task) Writer() io.Writer {
	if t == nil || t.out == nil {
		return io.Discard
	}
	return txt.NewChildWriter(t.capture(), t.appendLiveTail, t.setLiveOnlyPhase)
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

// boardSource is what the live board reads of this run's Tasks and
// collections.
type boardSource struct{ o *Run }

func (s boardSource) Pull() { s.o.pullRecordLocked() }

func (s boardSource) View(id record.TaskID) record.TaskSnapshot {
	return s.o.rowOf(id).view()
}

func (s boardSource) Snapshot(id record.TaskID) record.TaskSnapshot {
	return s.o.rowOf(id).snapshot()
}

func (s boardSource) Header(id string) record.TasksSnapshot {
	return s.o.containerStates[id].header()
}

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

// --- folded from snapshot.go ---

func (o *Run) snapshotLocked() snapshot {
	o.followRecordLocked()
	s := snapshot{
		Version:       o.version,
		OutputID:      o.outputID,
		Subject:       o.cfg.subject,
		Lines:         o.rec.Lines(),
		Actions:       record.CloneActions(asRecordActions(o.collectActionsLocked())),
		Timestamp:     o.cfg.clock.Now(),
		DryRun:        o.cfg.dryRun,
		Preview:       o.cfg.preview,
		DryRunSubject: o.cfg.dryRunHeaderText,
		Warnings:      o.rec.RunWarnings(),
		Facts:         o.rec.RunFacts(),
	}
	for _, col := range o.collections {
		s.Collections = append(s.Collections, col.snapshot())
	}
	// Root tasks not in a collection
	for _, t := range o.tasks {
		if t.collection() == nil {
			s.Tasks = append(s.Tasks, t.snapshot())
		}
	}
	s.Changes = o.rec.ChangeSnapshots()
	s.Plans = o.rec.PlanSnapshots()
	s.Messages = o.rec.Messages()
	if c, ok := o.rec.Conclusion(); ok {
		s.Conclusion = &c
	}
	return s
}

func (t *taskState) snapshot() taskSnapshot {
	s := t.view()
	s.Problems = record.CloneProblems(s.Problems)
	s.Warnings = record.CloneProblems(s.Warnings)
	s.Facts = record.CloneFacts(s.Facts)
	s.Verification = record.CloneVerificationDetails(s.Verification)
	s.Actions = record.CloneActions(s.Actions)
	s.Skipped = cloneTaxonomy(s.Skipped)
	s.Kept = cloneTaxonomy(s.Kept)
	if t.handle != nil {
		s.Tracked = t.handle.out.freshness.Tasks().Tracked(t.node.TaskID())
	}
	tail := record.LiveTailOf(s)
	tail.Lines = slices.Clone(tail.Lines)
	return record.WithLiveTail(s, tail)
}

// view is t's snapshot sharing t's slices: read it under o.mu and never
// let it escape, since the next mutation of t can change what it shows.
// It costs no allocation, so a live frame can classify every Task by its
// view and snapshot only the ones it shows.
func (t *taskState) view() taskSnapshot {
	colID := ""
	if t.collection() != nil {
		colID = t.collection().id
	}
	truth := t.node.Rec.Truth()
	base := taskSnapshot{
		ID:           t.id,
		Key:          t.node.Key(),
		Name:         t.name,
		State:        truth.State,
		Phase:        truth.Phase,
		ActivityAt:   truth.ActivityAt,
		Progress:     truth.Progress,
		Summary:      truth.Summary,
		Problems:     truth.Problems,
		Warnings:     truth.Warnings,
		Facts:        truth.Facts,
		Verification: truth.Verification,
		Actions:      truth.Actions,
		Skipped:      truth.Skipped,
		Kept:         truth.Kept,
		Collection:   colID,
		Declaration:  t.node.Declaration,
		DefineRan:    truth.DefineRan,
		Evidence:     truth.VerifyEvidence,

		Timing: t.timing,
	}
	return record.WithLiveTail(record.NewTaskSnapshot(base, t.board.FirstSeen(t.node.TaskID()), t.synthetic), t.board.Tail(t.node.TaskID()))
}

func cloneTaxonomy(in []taxonomyRecord) []taxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]taxonomyRecord, len(in))
	copy(out, in)
	for i := range out {
		if len(out[i].Causes) > 0 {
			out[i].Causes = append([]string(nil), out[i].Causes...)
		}
	}
	return out
}

// derivedState folds both this container's own tasks and its nested
// children (P3's recursive nesting) into one verdict: a nested Sequence or
// Group contributes exactly like one more task would, so a failure
// three levels deep still surfaces at the root header.
func (g *tasksState) derivedState() entityState {
	if g.node.BuilderFailed() {
		return failed
	}
	tasks, children := g.taskCount(), g.childStates()
	if tasks == 0 && len(children) == 0 {
		if g.node.BuilderNotStarted() {
			return notStarted
		}
		return empty
	}
	// A NotStarted child normally borrows its group's verdict from the
	// sibling that failed first, so it contributes nothing of its own. When
	// every child is NotStarted there is no such sibling — whatever stopped
	// the run was another subject entirely — and folding to Done rendered a
	// check over a subject that never ran.
	if len(children) == 0 && g.allTasksNotStarted(tasks) {
		return notStarted
	}
	v := foldStates(g.out.board.States(g.id))
	for _, child := range children {
		switch s := child.derivedState(); s {
		case empty:
		case notStarted:
			v.unresolved = true
		default:
			v.add(s)
		}
	}
	return v.state()
}

func (g *tasksState) allTasksNotStarted(tasks int) bool {
	return tasks > 0 && g.out.board.States(g.id).NotStarted == tasks
}

// foldStates is the verdict of a collection's direct Tasks alone.
func foldStates(s live.StateCounts) verdictFold {
	return verdictFold{
		running: s.Running > 0, failed: s.Failed > 0, refused: s.Refused > 0,
		cancelled: s.Cancelled > 0, unresolved: s.Unresolved > 0,
	}
}

// verdictFold accumulates member states into one container verdict. A
// Refused member counts like a Failed one does for the Tasks After the
// container (see stateOutcome): the container finished and did not
// succeed, so its header never reads Incomplete for it.
type verdictFold struct {
	running, failed, refused, cancelled, unresolved bool
}

func (v *verdictFold) add(s entityState) {
	switch s {
	case running:
		v.running = true
	case failed:
		v.failed = true
	case refused:
		v.refused = true
	case cancelled:
		v.cancelled = true
	case succeeded, record.Satisfied, excluded, notStarted:
	default:
		v.unresolved = true
	}
}

func (v verdictFold) state() entityState {
	switch {
	case v.running:
		return running
	case v.failed:
		return failed
	case v.refused:
		return refused
	case v.cancelled:
		return cancelled
	case v.unresolved:
		return incomplete
	default:
		return succeeded
	}
}

// displaySummary is g's Summary as its row shows it, given its derived
// state st: only when all children done/skipped successfully.
func (g *tasksState) displaySummary(st entityState) string {
	if st.Succeeded() && g.node.Rec.Summary() != "" && !g.hasWarnedOrFailedDescendant() {
		return g.node.Rec.Summary()
	}
	return ""
}

// hasWarnedOrFailedDescendant reports whether g or any nested child
// container carries a Failed/Cancelled task or a task with a Warn annotation
// (E2.5 finding 1): the group's own success summary must not paper over a
// warning or failure living several containers deep — the same suppression
// this method already gave Failed/Cancelled, restored and extended to
// Warnings.
func (g *tasksState) hasWarnedOrFailedDescendant() bool {
	if s := g.out.board.States(g.id); s.Failed > 0 || s.Cancelled > 0 || s.Warned > 0 {
		return true
	}
	for _, child := range g.childStates() {
		if child.hasWarnedOrFailedDescendant() {
			return true
		}
	}
	return false
}

func (o *Run) collectActionsLocked() []Action {
	seen := map[string]struct{}{}
	var out []Action
	add := func(list []Action) {
		for _, a := range list {
			k := record.ActionKey(asRecordAction(a))
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, a)
		}
	}
	for _, t := range o.tasks {
		add(asEvoActions(t.node.Rec.Actions()))
		// ZYS-848: a remedy attached via evo.Next(...) to an individual
		// Problem/warning (task.Problem(msg, evo.Next(...)),
		// task.Fail(msg, evo.Next(...))) must reach the run's own Next
		// steps the same way a task-level Next(...) call already does —
		// otherwise a remedy on one of several accumulated Problems is
		// invisible everywhere: writeProblem never renders p.Actions
		// inline (it is evidence, not a decision), and without this loop
		// it was silently dropped from the Conclusion's Next list too.
		truth := t.node.Rec.Truth()
		for _, p := range truth.Problems {
			add(asEvoActions(p.Actions))
		}
		for _, w := range truth.Warnings {
			add(asEvoActions(w.Actions))
		}
	}
	return out
}

func (g *tasksState) snapshot() tasksSnapshot {
	ts := g.header()
	for _, t := range g.taskStates() {
		ts.Tasks = append(ts.Tasks, t.snapshot())
	}
	for _, child := range g.childStates() {
		ts.Collections = append(ts.Collections, child.snapshot())
	}
	return ts
}

// header is g's snapshot without its children.
func (g *tasksState) header() tasksSnapshot {
	state := g.derivedState()
	return tasksSnapshot{
		ID:          g.id,
		Key:         g.node.Key(),
		Name:        g.name,
		State:       state,
		Summary:     g.displaySummary(state),
		Declaration: g.node.Declaration,
		Sequential:  g.node.Sequential,
	}
}

// --- folded from structured_events.go ---

// narratesEvents reports whether this Run streams §38 events (FormatJSONL).
// The Format is fixed at construction, so a caller reads it without o.mu:
// narration nobody reads costs neither a payload nor a trip through the
// Run's lock.
func (o *Run) narratesEvents() bool { return o.cfg.wireFormat == formatJSONL }

// emitWireEventLocked appends one §38 "evo.event" JSONL line for eventType,
// scoped to entityID (empty for a run-level recEvent) with payload as its
// domain fields. It is a no-op unless this Output is configured for
// FormatJSONL (spec §32.1: the event stream exists only in that Format) —
// a call site whose payload costs anything checks narratesEvents first.
//
// seq comes from the stream's own counter (wireSeq), not the journal's: the
// journal numbers events no line announces, which would leave gaps.
//
// A write failure is recorded rather than surfaced here (first one wins):
// this often runs deep inside the runtime's own lock, before any Task can
// meaningfully react. Finish folds wireEventErr into the misuse error it
// already returns, so "a later write fails; the Run then fails" without
// discarding the lines already written (see Finish's use of wireEventErr).
func (o *Run) emitWireEventLocked(eventType, entityID string, payload map[string]any) {
	if !o.narratesEvents() {
		return
	}
	o.wireSeq++
	ev := recEvent{
		Type:      eventType,
		Sequence:  o.wireSeq,
		Timestamp: o.cfg.clock.Now(),
		OutputID:  o.outputID,
		EntityID:  entityID,
		Payload:   payload,
	}
	if err := writeWireEventLocked(o.cfg.wireStream, ev); err != nil && o.wireEventErr == nil {
		o.wireEventErr = err
	}
}

// emitCollectionDeclaredLocked emits collection.declared (spec §38) for a
// newly declared Group/Sequence container, at top level or nested — the one
// call site Group, Sequence, and declareChildContainerLocked all share.
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

// --- folded from wireformat.go ---

// wireEvoVersion is the evo_version literal internal/wire's v2 encoder
// stamps onto every "evo.run" document. The engine package owns no version
// string of its own (root's PublishedRelease is the single source of
// truth, release.go) — SetWireEvoVersion lets the root package's package
// init pin it once, so there is exactly one place a release bump has to
// touch.
var wireEvoVersion = "dev"

// SetWireEvoVersion pins the evo_version literal FormatJSON/FormatJSONL
// stamp onto the v2 wire documents they emit. Called once, by the root
// package's init(), from evo.PublishedRelease.
func setWireEvoVersion(v string) { wireEvoVersion = v }

// writeWireRunLocked encodes conc as the final "evo.run" document and
// writes it plus a trailing newline to w (spec §53: "plus one trailing
// newline"). A nil w or nil conc is a no-op. Encode/write failures are
// wrapped in ErrRenderer, matching writeMachinePresentation's legacy-wire
// contract (spec §32.2: "a genuine Run failure, not an ignored logging
// error").
func writeWireRunLocked(w io.Writer, conc Conclusion) error {
	if w == nil {
		return nil
	}
	body, err := wire.EncodeRun(record.Result{Conclusion: asRecordConclusion(conc)}, wireEvoVersion)
	if err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}

// writeWireEventLocked encodes e as one "evo.event" JSONL line and writes
// it to w — the FormatJSONL counterpart of writeStreamJSONLocked's legacy
// "0.3" streamed-JSON line, called from emitWireEventLocked
// (structured_events.go). A write failure here does not stop the stream —
// earlier lines already written stay valid (spec §32.2: "Earlier valid
// lines remain valid if a later write fails") — but it is returned so the
// caller can latch it as this Run's first wire-write failure and fail the
// Run at Finish (spec §32.2: "... the Run then fails"), instead of the
// error silently vanishing.
func writeWireEventLocked(w io.Writer, e recEvent) error {
	if w == nil {
		return nil
	}
	row, err := wire.EncodeEvent(e)
	if err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	row = append(row, '\n')
	if _, err := w.Write(row); err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
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
