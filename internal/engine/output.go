package engine

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/record"
	renderplain "github.com/zachbornheimer/evident-output/internal/render/plain"
	"github.com/zachbornheimer/evident-output/internal/terminal"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Output is the aggregate root for one command's presentation lifecycle.
type Output struct {
	// mu guards the render state. Its sections hold the run's notifications,
	// so the engine's listener hears a section's writes once the mutex is free.
	mu record.Mutex
	// facade holds this Output's public wrapper (see FacadeSlot).
	facade FacadeSlot

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
	closed  bool
	// closing is non-nil once a Close call claimed the teardown; it closes
	// when that teardown ends, so a concurrent Close waits instead of
	// tearing down twice.
	closing   chan struct{}
	finishing bool
	finished  bool
	armed     bool // set by arm(): live surface may paint before any entity exists
	// misuseMu guards misuse, misuseSubject and misuseRejectedSummary. It is
	// a leaf lock, taken without Output.mu by the graph's own goroutines.
	misuseMu sync.Mutex
	misuse   error
	// misuseSubject names the task/entity the first recorded misuse happened
	// on, when known — Finish renders one line naming it whenever that misuse
	// is about to change the exit code, so a caller never sees an exit code
	// contradict what the printed band showed (beginner-1).
	misuseSubject string
	// misuseRejectedSummary is the summary text a second terminal verb
	// (Fail/Block/Warn/Cancel/Skipped) tried to attach to an
	// already-resolved task, captured on the first recorded misuse only —
	// the same "first ever recorded" scope as misuseSubject. Empty when the
	// rejected call carried no summary, or the first misuse wasn't this kind
	// (release-gate round 5 finding 4: the band's severity otherwise has no
	// visible cause beyond "was already resolved").
	misuseRejectedSummary string
	live                  *liveEngine

	// workspaceDir is the process working directory, captured once on first
	// use by File (workspace in file.go) so relative paths resolve
	// consistently even if the process CWD changes mid-Run (§8.1).
	workspaceDir string

	tasks []*taskState
	// rootTasks are the tasks outside every collection, in declaration
	// order: the ones a live frame walks beside the collections.
	rootTasks   []*taskState
	collections []*tasksState
	// rec holds the run's truth: its event journal, ledger, history lines,
	// messages, diagnostics, run-scoped facts and warnings, and Conclusion.
	rec *record.Run

	// slice 6: wireSeq is the second event sequence counter, beside the record
	// journal's. The two streams number independently in the goldens, so they
	// stay apart until project/wire owns both encoders.
	// wireSeq/wireEventErr back the §38 "evo.event" JSONL stream
	// (structured_events.go's emitWireEventLocked) — a counter and
	// first-write-failure latch independent of the legacy events journal
	// above.
	wireSeq      uint64
	wireEventErr error

	// graph is the declared shape of the run: identity, keys and numbering.
	graph *graph.Graph
	// taskStates and containerStates are engine's own state for each node the
	// graph declared, keyed by the node's ID.
	taskStates      map[string]*taskState
	containerStates map[string]*tasksState
	// rootColumn is the alignment width for root Task rows.
	rootColumn rootColumn
	// Progressive durable emission (§17.5: terminal outcomes render immediately).
	// Finish only appends residual (unemitted entities + conclusion).
	linesEmitted int

	// durableRowsEmitted counts writeDurableTextLocked calls with non-empty
	// text — the one shared choke point every durable line goes through.
	// DeclareDryRun (I8) uses it to bound itself: once any row has
	// streamed, flipping DryRun retroactively would leave earlier rows not
	// reflecting it, so DeclareDryRun after that point is misuse.
	durableRowsEmitted int

	// debugPaneActive is true once a Debug record was subject to pane
	// presentation while interactive — either projected onto the live
	// rolling pane, or (dual-stream construction) routed to Diagnostics only
	// while pane presentation was still in effect. Controls failure
	// diagnostic-tail eligibility; not "the pane rendered this record".
	debugPaneActive bool

	// Print/Printf/Println line buffer.
	pendingPrint strings.Builder
	pendingVis   Visibility // visibility of the current pending fragment

	// manifestStore is this Run's exclusive handle on the reconciliation
	// manifest (spec §11.3), opened lazily by the first evo.File call
	// (manifestFor) the same way workspaceDir is captured lazily on first
	// use. manifestOpenErr/manifestOpened distinguish "not yet opened" from
	// "opened and failed" so a later call does not retry a failed open.
	manifestStore *manifest.Store
	// manifestFinishErr is the failure of the save-and-release Finish did;
	// Close returns it so a write failure is never dropped.
	manifestFinishErr error
	// manifestOpening serializes manifestFor's first open without holding
	// mu across the blocking lock wait (see manifestFor).
	manifestOpening       sync.Mutex
	manifestOpened        bool
	manifestOpenErr       error
	manifestWarningIssued bool
	// manifestUnsavedIssued records that the run already warned its
	// manifest was not saved, so a failed write is stated once.
	manifestUnsavedIssued bool
	// manifestApp is this Run's application record, computed once and
	// reused on every Task commit (spec §11.2/§11.3).
	manifestApp     manifest.ApplicationRecord
	manifestAppDone bool
	// manifestClaims records which Task first claimed each canonical File/
	// Exec output path in this Run (spec §11.4/§8.3): a second Task
	// claiming the same path is a producer conflict.
	manifestClaims map[string]string
}

func newOutput(subject string, options ...Option) *Output {
	cfg := config{
		subject:         subject,
		clock:           systemClock{},
		visibilityDelay: defaultVisibilityDelay,
		maxFrameRate:    defaultMaxFrameRate,
		width:           defaultWidth,
		debugLevel:      LevelInfo,
		redactor:        process.NoopRedactor{},
		maxEntities:     defaultMaxEntities,
		verbosity:       VerbosityNormal,
		processRunner:   process.System(),
		fileFS:          osFileFS{},
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
	// Config's own to(c.Stdout)/to(c.Stderr) wiring (Config.Terminal path,
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
	o := &Output{
		cfg:             cfg,
		rec:             run,
		outputID:        "out_1",
		taskStates:      make(map[string]*taskState),
		containerStates: make(map[string]*tasksState),
	}
	o.graph = graph.New(run, graph.WithMaxEntities(cfg.maxEntities), graph.WithMaxConcurrency(cfg.maxConcurrency), graph.WithMisuseSink(misuseSink{o: o}))
	o.mu.Bind(run)
	run.SetListener(outputListener{o: o})
	// Stable-enough id for a process-local output instance.
	o.outputID = o.graph.NextID("out")
	o.startedAt = o.cfg.clock.Now()
	o.appendEventLocked(Event{Type: "output.started", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventRunStarted, "", nil)
	if terminalWithoutSink {
		o.recordMisuse(ErrTerminalWithoutSink)
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

// DeclareDryRun switches this run into dry-run mode after construction — a
// bounded late setter (I8): calling it once any durable row has already
// streamed is misuse (ErrDryRunDeclaredLate), since those earlier rows
// would not reflect the switch. There is no argv-sniffing helper; the
// caller decides (e.g. from a flag parsed after Init) and calls this
// explicitly, before any Task/Print/Confirm call. A no-op when the run is
// already dry-run.
func (o *Output) declareDryRun() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if o.cfg.dryRun {
		return
	}
	if o.durableRowsEmitted > 0 {
		o.recordMisuse(ErrDryRunDeclaredLate)
		return
	}
	o.cfg.dryRun = true
	o.emitPlannedHeaderLocked()
}

// emitPlannedHeaderLocked writes the planned run's opening announcement
// immediately, once, through the same durable-write path
// (writeDurableTextLocked) every other library-owned line uses.
func (o *Output) emitPlannedHeaderLocked() {
	var b strings.Builder
	renderplain.WritePlannedHeader(&b, !o.cfg.noColor, o.cfg.preview, o.cfg.dryRunHeaderText)
	o.writeDurableTextLocked(b.String())
}

// attachVerificationLocked appends details onto taskID's own running
// record — evo.File/evo.Exec's per-attribute reconciliation evidence
// (spec §2/§8.2), recorded before the task resolves so it is already
// present by the time Fail's terminal verb reads the task's state
// (the same "annotate before terminal" timing Fact and warning-severity Problem require). A no-op
// once the task has already resolved or does not exist.
func (o *Output) attachVerificationLocked(taskID string, details []core.VerificationDetail) {
	st := o.taskStates[taskID]
	if st == nil || core.IsTerminalTask(st.rec.State()) {
		return
	}
	stored := st.rec.AttachVerification(details)
	st.markFiling()
	// Emit the sanitized copy evo.run projects, so JSONL and JSON agree.
	for _, d := range stored {
		o.emitWireEventLocked(wire.EventVerificationObserved, taskID, wire.ToVerificationDoc(d).EventPayload())
	}
}

// promoteRunningLocked transitions a Pending task to Running on its first
// unit of evidence (Phase/Progress/Advance/Bytes/Step/Writer
// write, or a work callback starting — see promoteRunningForActivity).
// For a sequential collection (Sequence), it records misuse when a sibling is
// already Running, enforcing the heart contract "one Running child"
// (evo-rec.md) — callers still get the transition; Strict mode is what
// escalates the violation to a panic. A plain Group collection
// documents its children as independent (worker-pool fan-out is a
// supported, concurrency-safe pattern there), so it is not policed.
func (o *Output) promoteRunningLocked(st *taskState) {
	if col := st.collection; col != nil && col.sequential {
		col.runningSteps = slices.DeleteFunc(col.runningSteps, func(s *taskState) bool { return s.rec.State() != Running })
		if len(col.runningSteps) > 0 {
			o.recordMisuse(ErrConcurrentRunning)
		}
		col.runningSteps = append(col.runningSteps, st)
	}
	st.rec.Transition(Running)
	st.markFiling()
	o.armPlainHeartbeatLocked(st, o.cfg.clock.Now())
	// Every promoteRunningLocked call site already guards on st.state ==
	// Pending before calling it, and this line immediately advances past
	// Pending — so task.started fires exactly once per task's lifetime.
	o.emitWireEventLocked(wire.EventTaskStarted, st.id, nil)
}

func (o *Output) bumpLocked() {
	o.version++
}
