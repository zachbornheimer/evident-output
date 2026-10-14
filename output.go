package evo

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/present"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// Run is the root of one invocation, handed to the application body by Main and Execute. It is a Container:
// declare root work with Task, Group and Sequence, add run-scoped information with Fact and Summary, ask the
// user with Confirm, and write free text with Text. Evo decides when declared work runs and how it is shown;
// the body only declares it and returns.
type Run struct {
	// mu guards the render state. Its sections hold the run's notifications,
	// so the engine's listener hears a section's writes once the mutex is free.
	mu  record.Mutex
	cfg config

	outputID string
	// startedAt is the wire envelope's started_at (§35), read back at Finish, never re-derived.
	startedAt time.Time
	closed    bool
	// closing is non-nil once a Close claimed the teardown and closes when it
	// ends, so a concurrent Close waits.
	closing chan struct{}
	// confirmTurn lets one Confirm at a time own the terminal.
	confirmTurn *terminal.Turn
	finishing   bool
	finished    bool
	// misuse records the first API misuse. Graph workers call it without Run.mu;
	// a strict recorder panics after recording, never under graph.mu.
	misuse *misuse.Recorder
	// show presents the run: rows, live region, durable stream, evo.event stream.
	show *present.Presenter

	// changes performs File, Exec, Patch and Effect and owns the workspace paths resolve against.
	changes *change.Run

	tasks []*taskState
	// rootTasks are the tasks outside every collection, in declaration order.
	rootTasks   []*taskState
	collections []*tasksState
	// rec holds the run's truth: journal, ledger, lines, messages, facts, warnings, Conclusion.
	rec *record.Run

	graph *graph.Graph
	// taskStates and containerStates hold engine state per graph node, keyed by node ID.
	taskStates      map[string]*taskState
	containerStates map[string]*tasksState

	// resultText is what ResultWriter received in JSON, whose stdout carries one document the text rides in.
	resultText strings.Builder
	// printLines assembles Print fragments into message lines.
	printLines *txt.PrintLines

	// freshness is this Run's lazily opened reconciliation manifest (§11.3) and
	// what it remembers about each Task.
	freshness  *freshness.Run
	cleanStops graph.CleanStops
}

// runsStarted counts the runs this process has started; it keeps two runs on one frozen Clock apart.
var runsStarted atomic.Uint64

// newRun builds the run cfg describes: its graph, freshness, changes and every projection.
func newRun(cfg config) *Run {
	resolveGlyphProfileLocked(&cfg)
	run := record.NewRun()
	o := &Run{
		cfg:             cfg,
		rec:             run,
		misuse:          misuse.New(),
		confirmTurn:     terminal.NewTurn(),
		taskStates:      make(map[string]*taskState),
		containerStates: make(map[string]*tasksState),
	}
	host := runHost{out: o}
	o.graph = graph.New(run, graph.WithMaxEntities(cfg.maxEntities), graph.WithMaxConcurrency(cfg.maxConcurrency), graph.WithMisuseSink(o.misuse), graph.WithHost(host))
	o.freshness = freshness.NewRun(o.graph, freshness.RunConfig{Where: o.manifestConfig, Closed: errClosed, DryRun: host.DryRun, Narration: host})
	o.changes = change.NewRun(o.newChangeHost(), o.graph)
	o.startedAt = cfg.clock.Now()
	o.outputID = record.NewRunID(o.startedAt, process.PID(), runsStarted.Add(1))
	o.show = present.New(present.Config{
		Run: run, Lock: &o.mu, Clock: cfg.clock, Host: presentHost{o},
		RunID: o.outputID, Events: cfg.eventStream(),
		Surface: liveSurfaceOf(cfg), VisibilityDelay: cfg.visibilityDelay, MaxFrameRate: cfg.maxFrameRate,
		Style: o.humanStyle(), Subject: cfg.subject,
		Sinks: humanSinks{o},
	})
	o.printLines = txt.NewPrintLines(o.emitMessageLocked)
	o.mu.Bind(run)
	o.appendEventLocked(recEvent{Type: "output.started", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventRunStarted, "", wire.RunStartedPayload{Name: o.cfg.subject}.EventPayload())
	if cfg.dryRun {
		// Announce before any task can reach the durable stream; unlocked is safe, o has not been returned yet.
		o.show.PlannedHeader()
	}
	return o
}

func (o *Run) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: o.changes.Workspace()}
}

// promoteRunningLocked moves a Pending task to Running on its first unit of evidence (Phase, Progress,
// Advance, Bytes, Step, Writer). A work callback starting is announced by taskStarted: the claim moved the row.
func (o *Run) promoteRunningLocked(st *taskState) {
	st.node.MarkRunning()
	o.announceRunningLocked(st)
}

// announceRunningLocked is the render state a Task owes for becoming Running, answered once per Task.
// A Sequence records misuse when a sibling is already Running; a Group's children are not policed.
func (o *Run) announceRunningLocked(st *taskState) {
	if !o.show.Started(st.node.TaskID()) {
		return
	}
	if seq := st.node.Parent; seq != nil && seq.Sequential && seq.StepStarted(st.node) {
		o.recordMisuse(errConcurrentRunning)
	}
}

func (o *Run) bumpLocked() { o.show.Bump() }

// appendEventLocked journals e in the order things happened: the record is followed first, so a settle a graph
// goroutine made on its own is journaled before an event this section writes after it.
func (o *Run) appendEventLocked(e recEvent) {
	o.followRecordLocked()
	e.Timestamp = o.cfg.clock.Now()
	e.SchemaVersion = eventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = o.outputID
	}
	o.graph.Root().Journal(e, o.cfg.maxEvents)
}

// followRecordLocked brings every projection up to date with the record. Callers hold o.mu.
func (o *Run) followRecordLocked() { o.show.Follow() }

type taskState struct {
	id string
	// node is the graph's declaration of this Task; it owns the stable key and scheduling.
	node   *graph.Task
	name   string
	handle *Task
	// refused marks a Task whose declaration was refused as misuse (Compute on a Task with a Verify, Define(nil)). It never
	// runs, and the misuse line already says why, so Finish settles it without a "forgot to resolve" row.
	refused bool
}

type tasksState struct {
	id string
	// out is the Run this container belongs to; reading its live index first follows the record.
	out *Run
	// node is the graph's declaration of this Group/Sequence (§3.1 stable key).
	node   *graph.Container
	name   string
	handle *Group
}

// taskStates are the render states of the Tasks g declared directly, in declaration order.
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

// appendTaskLocked gives st a row in every projection and announces it; synthetic marks the run's own ending Task.
func (o *Run) appendTaskLocked(st *taskState, synthetic bool) {
	in := ""
	if col := st.collection(); col != nil {
		in = col.id
	}
	row := txt.DeclaredTask{ID: st.node.TaskID(), Node: st.node, Name: st.name, Declaration: st.node.Declaration, Rec: st.node.Rec, Synthetic: synthetic}
	if !synthetic {
		row.Timing.Declared = o.cfg.clock.Now()
	}
	o.tasks = append(o.tasks, st)
	if in == "" {
		o.rootTasks = append(o.rootTasks, st)
	}
	o.show.DeclareTask(row, in)
}

// arm marks the live surface ready to paint before any entity is declared (first-paint rule). Idempotent.
func (o *Run) arm() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.show.Arm()
}

type presentHost struct{ o *Run }

func (h presentHost) Ended() bool        { return h.o.closed || h.o.finished }
func (h presentHost) Width() int         { return h.o.cfg.width }
func (h presentHost) Resize(width int)   { h.o.cfg.width = width }
func (h presentHost) Journal(e recEvent) { h.o.appendEventLocked(e) }
func (h presentHost) Tracked(id record.TaskID) record.TaskOperations {
	return h.o.freshness.Tasks().Tracked(id)
}

// snapshotLocked is the run as every projection shows it now. Callers hold o.mu.
func (o *Run) snapshotLocked() snapshot {
	return o.show.Snapshot(snapshot{
		OutputID:  o.outputID,
		Subject:   o.cfg.subject,
		Lines:     o.rec.Lines(),
		Timestamp: o.cfg.clock.Now(),
		DryRun:    o.cfg.dryRun,
		Warnings:  o.rec.RunWarnings(),
		Facts:     o.rec.RunFacts(),
	})
}

// emitWireEventLocked writes one §38 "evo.event" line; a run that streams no events writes nothing.
func (o *Run) emitWireEventLocked(eventType, entityID string, payload map[string]any) {
	o.show.Events.Emit(eventType, entityID, payload)
}

// Output selects the presentation encoding (CONTRACT §6).
type Output int

const (
	Human Output = iota + 1
	Plain
	JSON
	JSONL
)

// Environment keys honored at run start when the matching Config field is unset.
const (
	envKeyOutput     = "EVO_OUTPUT"
	envKeyVerbose    = "EVO_VERBOSE"
	envKeyDebug      = "EVO_DEBUG"
	envKeyNoColor    = "NO_COLOR"
	envKeyForceColor = "FORCE_COLOR"
	envKeyLocaleAll  = "LC_ALL"
	envKeyLocaleType = "LC_CTYPE"
	envKeyLang       = "LANG"
)

const (
	envForceColorOff = "0"
	envVerboseOn     = "1"
)

// envOutputs are EVO_OUTPUT's values; any other value leaves Output to terminal inference.
var envOutputs = map[string]Output{"human": Human, "plain": Plain, "json": JSON, "jsonl": JSONL}

// envDebugLevels are EVO_DEBUG's values.
var envDebugLevels = map[string]logLevel{"info": levelInfo, "debug": levelDebug, "warn": levelWarn, "error": levelError}

var (
	lookupEnvMu sync.RWMutex
	lookupEnvFn = process.Env
)

func lookupEnv(key string) string {
	lookupEnvMu.RLock()
	fn := lookupEnvFn
	lookupEnvMu.RUnlock()
	if fn == nil {
		return process.Env(key)
	}
	return fn(key)
}

// outputFromEnv is the Output EVO_OUTPUT's value v names, zero when it names none.
func outputFromEnv(v string) Output { return envOutputs[strings.TrimSpace(v)] }

// forceColorRequested reports whether FORCE_COLOR asks for color; NO_COLOR wins, so it is checked first.
func forceColorRequested() bool {
	if lookupEnv(envKeyNoColor) != "" {
		return false
	}
	force := strings.TrimSpace(lookupEnv(envKeyForceColor))
	return force != "" && force != envForceColorOff
}

// parseDebugEnv is the journal level EVO_DEBUG's value v names.
func parseDebugEnv(v string) (logLevel, bool) {
	lvl, ok := envDebugLevels[strings.TrimSpace(v)]
	return lvl, ok
}
