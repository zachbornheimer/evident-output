package engine

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/render"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Output is the aggregate root for one command's presentation lifecycle.
type Output struct {
	mu sync.Mutex
	// facade holds this Output's public wrapper (see FacadeSlot).
	facade FacadeSlot

	cfg config

	outputID string
	// startedAt is the wire v2 envelope's run_id-adjacent "started_at"
	// (spec §35) — captured once at construction through the Clock facade,
	// read back (never re-derived) at Finish.
	startedAt time.Time
	idSeq     uint64
	declSeq   int
	version   uint64
	closed    bool
	// closing is non-nil once a Close call claimed the teardown; it closes
	// when that teardown ends, so a concurrent Close waits instead of
	// tearing down twice.
	closing   chan struct{}
	finishing bool
	finished  bool
	armed     bool // set by arm(): live surface may paint before any entity exists
	misuse    error
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
	conclusion            *Conclusion
	live                  *liveEngine

	// workspaceDir is the process working directory, captured once on first
	// use by File (workspace in file.go) so relative paths resolve
	// consistently even if the process CWD changes mid-Run (§8.1).
	workspaceDir string

	tasks       []*taskState
	collections []*tasksState
	// changes and plans are the run's [changed] and [planned] sections, in
	// ledger order (ledger_section.go).
	changes []*ledgerSection
	plans   []*ledgerSection
	// ledger finds a section by its owning Task or shown name without
	// rescanning (ledger_order.go).
	ledger  ledgerIndex
	lines   []string
	actions []Action
	journal journal

	// wireSeq/wireEventErr back the §38 "evo.event" JSONL stream
	// (structured_events.go's emitWireEventLocked) — a counter and
	// first-write-failure latch independent of the legacy events journal
	// above.
	wireSeq      uint64
	wireEventErr error

	taskByRef  map[string]*taskState
	tasksByRef map[string]*tasksState
	// rootColumn is the alignment width for root Task rows.
	rootColumn rootColumn
	keys       map[string]struct{}

	// rootNames holds the names root Tasks, Groups, and Sequences claimed
	// (§3.1); see siblingsLocked.
	rootNames siblings
	// namedReasons backs get-or-create identity for evo.Reason: repeated calls
	// with the same name (inline or lifted to a var) merge into one bucket.
	// Also unrelated to §3.1 — a taxonomy Reason is not a declared entity.
	namedReasons map[string]TaxonomyReason

	// ctx is the run's own cancellation signal — the thing a callback doing
	// I/O selects on. cancelRun trips it on interrupt and on Close, so no
	// callback can outlive the run that owns it.
	ctx       context.Context
	cancelRun context.CancelFunc
	// cancelCause names who stopped the run ("by user" for a signal). It
	// becomes the cancelled Conclusion's Explanation, so the band and the
	// JSON document state the same cause.
	cancelCause string

	// sched is the run's scheduling state (scheduler_state.go).
	sched scheduler

	// confirmAbort holds one abort channel per pending Confirm gate, keyed by
	// item id, so cancelActive can unblock Confirm's stdin read and resolve
	// the gate as Cancelled (not Blocked "declined") on SIGINT/SIGTERM.
	confirmAbort map[string]chan struct{}

	// Progressive durable emission (§17.5: terminal outcomes render immediately).
	// Finish only appends residual (unemitted entities + conclusion).
	linesEmitted int

	// durableRowsEmitted counts writeDurableTextLocked calls with non-empty
	// text — the one shared choke point every durable line goes through.
	// DeclareDryRun (I8) uses it to bound itself: once any row has
	// streamed, flipping DryRun retroactively would leave earlier rows not
	// reflecting it, so DeclareDryRun after that point is misuse.
	durableRowsEmitted int

	// Structured debug journal (§21.3). lines[] still holds history-format strings
	// for FinalPlain / residual compatibility.
	debugRecords []debugRecord
	// debugPaneActive is true once a Debug record was subject to pane
	// presentation while interactive — either projected onto the live
	// rolling pane, or (dual-stream construction) routed to Diagnostics only
	// while pane presentation was still in effect. Controls failure
	// diagnostic-tail eligibility; not "the pane rendered this record".
	debugPaneActive bool

	// Print/Printf/Println line buffers and canonical messages.
	pendingPrint strings.Builder
	pendingVis   Visibility // visibility of the current pending fragment
	messages     []messageState

	// runWarnings/runFacts accumulate Output.Warn/Output.Fact's run-scoped
	// annotations (P8) — the same "annotate, never resolve" contract a
	// task's warnings/facts have, scoped to the run itself instead of one
	// task.
	runWarnings []Problem
	runFacts    []FactRecord

	// manifestStore is this Run's exclusive handle on the reconciliation
	// manifest (spec §11.3), opened lazily by the first evo.File call
	// (manifestFor) the same way workspaceDir is captured lazily on first
	// use. manifestOpenErr/manifestOpened distinguish "not yet opened" from
	// "opened and failed" so a later call does not retry a failed open.
	manifestStore         *manifest.Store
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
	// outputGates is the freshness barrier (spec §11.6/§64): claiming a
	// canonical output path opens a gate here; a later operation
	// consulting that same path as a Basis input waits on it until the
	// producing operation settles, without the scheduler inferring any
	// ordering edge from the data relationship itself.
	outputGates map[string]chan struct{}
}

func newOutput(subject string, options ...Option) *Output {
	cfg := config{
		subject:         subject,
		clock:           systemClock{},
		visibilityDelay: defaultVisibilityDelay,
		maxFrameRate:    defaultMaxFrameRate,
		width:           defaultWidth,
		debugLevel:      LevelInfo,
		redactor:        noopRedactor{},
		maxEntities:     defaultMaxEntities,
		verbosity:       VerbosityNormal,
		processRunner:   osProcessRunner{},
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
		cfg.primary = os.Stdout
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
	runCtx, cancelRun := context.WithCancel(context.Background())
	o := &Output{
		cfg:        cfg,
		outputID:   "out_1",
		taskByRef:  make(map[string]*taskState),
		tasksByRef: make(map[string]*tasksState),
		keys:       make(map[string]struct{}),
		ctx:        runCtx,
		cancelRun:  cancelRun,
	}
	// Stable-enough id for a process-local output instance.
	o.outputID = o.nextID("out")
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
	render.WritePlannedHeader(&b, !o.cfg.noColor, o.cfg.preview, o.cfg.dryRunHeaderText)
	o.writeDurableTextLocked(b.String())
}

// attachVerificationLocked appends details onto taskID's own running
// record — evo.File/evo.Exec's per-attribute reconciliation evidence
// (spec §2/§8.2), recorded before the task resolves so it is already
// present by the time Fail/Failf's terminal verb reads the task's state
// (the same "annotate before terminal" timing Fact/Warn require). A no-op
// once the task has already resolved or does not exist.
func (o *Output) attachVerificationLocked(taskID string, details []core.VerificationDetail) {
	st := o.taskByRef[taskID]
	if st == nil || core.IsTerminalTask(st.state) {
		return
	}
	stored := core.StoreVerificationDetails(details)
	st.verification = append(st.verification, stored...)
	// Emit the sanitized copy evo.run projects, so JSONL and JSON agree.
	for _, d := range stored {
		o.emitWireEventLocked(wire.EventVerificationObserved, taskID, wire.ToVerificationDoc(d).EventPayload())
	}
}

// promoteRunningLocked transitions a Pending task to Running on its first
// unit of evidence (Phase/Progress/Advance/Bytes/Writer
// write, or a work callback starting — see promoteRunningForActivity).
// For a sequential collection (Sequence), it records misuse when a sibling is
// already Running, enforcing the heart contract "one Running child"
// (evo-rec.md) — callers still get the transition; Strict mode is what
// escalates the violation to a panic. A plain Group collection
// documents its children as independent (worker-pool fan-out is a
// supported, concurrency-safe pattern there), so it is not policed.
func (o *Output) promoteRunningLocked(st *taskState) {
	if col := st.collection; col != nil && col.sequential {
		col.runningSteps = slices.DeleteFunc(col.runningSteps, func(s *taskState) bool { return s.state != Running })
		if len(col.runningSteps) > 0 {
			o.recordMisuse(ErrConcurrentRunning)
		}
		col.runningSteps = append(col.runningSteps, st)
	}
	from := st.state
	st.state = Running
	st.censusMoved(from)
	o.armPlainHeartbeatLocked(st, o.cfg.clock.Now())
	// Every promoteRunningLocked call site already guards on st.state ==
	// Pending before calling it, and this line immediately advances past
	// Pending — so task.started fires exactly once per task's lifetime.
	o.emitWireEventLocked(wire.EventTaskStarted, st.id, nil)
}

func (o *Output) bumpLocked() {
	o.version++
}
