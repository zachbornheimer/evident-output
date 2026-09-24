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
	// (Done/Fail/Block/Warn/Cancel/Skip) tried to attach to an
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

	// rootNames holds, per declaration scope, the names root Tasks, Groups,
	// and Sequences claimed (§3.1); see siblingsLocked.
	rootNames map[string]*siblings
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

type taskState struct {
	id          string
	key         string // optional stable machine key (platform ID)
	name        string
	state       EntityState
	phase       string
	progress    Progress
	summary     string
	problems    []Problem
	actions     []Action
	collection  *tasksState
	declaration int
	handle      *TaskHandle
	// sched is where this Task stands with the scheduler.
	sched taskSchedule

	// activityAt is the domain-clock time of the most recent Phase, Progress,
	// or work-callback-starting call — kept for the public
	// ActivityAt snapshot field and for Sequence's "one Running child"
	// bookkeeping. P5's elapsed-time render clock no longer reads it (see
	// elapsedAfter in live.go): the render anchor is liveFirstSeenAt alone,
	// so a fresh Phase/Progress call never restarts the elapsed suffix.
	activityAt time.Time

	// liveFirstSeenAt is the domain-clock time this task was first actually
	// painted in the live region (see stampLiveFirstSeenLocked in live.go) —
	// the one elapsed-time anchor every row's heartbeat suffix reads (P5),
	// including a Pending task, which never calls Phase/Progress.
	liveFirstSeenAt time.Time

	// heartbeat is the §40 plain-mode durable heartbeat's state.
	heartbeat plainHeartbeat

	// capture is the get-or-create sink shared by Task.Capture and PhaseWriter
	// so child-process evidence recorded via either path lands in one ring and
	// DetailTail sees it after Fail.
	evidence *evidence

	// skipped/kept hold disposition taxonomy accumulated by Skipped/Kept —
	// the model that "- skipped N (...)" / "! kept N (...)" are derived from
	// at render time, never a hand-built summary string. Disposition side of
	// the model, not the mutation ledger (Plan/Changes).
	skipped []TaxonomyRecord
	kept    []TaxonomyRecord

	// Emission bookkeeping so terminal standalone tasks stream in plain mode
	// on resolve (P2).
	coreEmitted bool

	// synthetic marks a task the library invented to carry an output-level
	// outcome (Output.Failf/Cancel's synthetic "command" task) rather than
	// one the caller declared. shouldSuppressRepeatedCondition (I2) must
	// never drop the standalone conclusion band for one of these: it is the
	// only place the run's outcome is ever stated, unlike a caller-declared
	// Task whose own row already says the same thing.
	synthetic bool

	// warnings accumulates TaskHandle.Warn's annotations (P2: warnings
	// annotate lifecycle, they never replace it — Warn does not itself
	// resolve the task). A task with warnings but no terminal verb by
	// Finish auto-resolves Done (see hasRecordedEffectLocked's amnesty
	// siblings in Finish).
	warnings []Problem
	// facts accumulates TaskHandle.Fact's discovered-information annotations
	// (P8) — info severity, the same "annotate, never resolve" contract
	// warnings has at warning severity.
	facts []FactRecord
	// verification accumulates every core.VerificationDetail an evo.File/
	// evo.Exec operation this Task ran recorded (spec §2/§8.2) — attached
	// before the task resolves, the same "annotate before terminal" timing
	// facts/warnings already require.
	verification []core.VerificationDetail

	// effectDenials counts the times this task's own mutation callback
	// resolved the row as something other than Done while an Effect ran, so
	// that Effect's work must not reach the ledger (see deniesItsOwnEffect).
	// Every in-flight Effect compares it at entry and exit.
	effectDenials int
	// effectsInFlight counts evo.Effect callbacks currently running for this
	// task; a non-Done resolution while one runs disowns that Effect.
	effectsInFlight int
	// verifiers holds TaskHandle.Verify's registered pre/post-Define
	// observation checks, ANDed in registration order (§9.1). Must be
	// registered before Define — see Verify.
	verifiers []verifierFunc
	// resolution names why this Task settled successfully (§29/§30);
	// ResolutionNoWork is the default until Define's own execution wiring
	// sets it to ResolutionExecuted/ResolutionAlreadySatisfied.
	resolution Resolution
	// verifyEvidence preserves both Verify observation phases (§30).
	verifyEvidence TaskEvidence
	// workErr is the callback's own return value, kept so TaskHandle.Wait
	// returns exactly what the work returned rather than a state guess.
	workErr error
	// proposed holds a caller's unratified success claim on a submitted task
	// until the callback's return value confirms or contradicts it.
	proposed *proposedOutcome
	doneOnce sync.Once
	doneCh   chan struct{}

	// plainStream is what plain progressive streaming already emitted for
	// this still-Running standalone task.
	plainStream plainStreamMark

	// manifestOps accumulates this Task's tracked operation records for the
	// current Run (spec §11.3-11.5): one entry per evo.File/evo.Exec call
	// that participated in manifest tracking, appended in call order (the
	// same order manifest.Store.Operation's ordinal indexes into). Never
	// populated during dry-run — dry-run commits nothing (§8.2).
	manifestOps []manifest.OperationRecord
}

type tasksState struct {
	id string
	// key is the §3.1 stable machine identity for this Group/Sequence: the
	// default kind+parent-key+normalized-name derivation, computed once at
	// declaration (see declareContainerLocked).
	key         string
	name        string
	summary     string
	tasks       []*taskState
	declaration int
	handle      *GroupHandle

	// names holds the names this container's child Tasks and containers
	// claimed (§3.1); see siblings.
	names siblings

	// sequential marks a Sequence: children are chained in declaration
	// order. A Group's children are independent and may overlap.
	sequential bool
	// runningSteps holds the children promoteRunningLocked moved to
	// Running that may still be Running (pruned on each promotion), so the
	// "one Running child" check never rescans every step.
	runningSteps []*taskState

	// children holds nested containers declared via Group.Group,
	// Group.Sequence, Sequence.Group, or Sequence.Sequence (P3's recursive
	// nesting) — a container's derived state and
	// rendering fold its children in exactly the way it folds its own
	// tasks.
	children []*tasksState

	// parent is the container this one is nested in, nil at the root.
	parent *tasksState
	// entry is what everything declared in this container starts after:
	// the step before it when it is a step of a Sequence (see
	// nextStepPreds).
	entry []predecessor
	// lastStep is, for a Sequence, what its next step starts after: the
	// one step declared most recently, Task or nested collection.
	lastStep []predecessor
	// stoppedAfter is, for a Sequence, the declaration of the earliest step
	// a failure already stopped its later steps after (0: none yet).
	stoppedAfter int
	// tally counts this container's descendant Tasks by outcome, for the
	// Tasks that run After it.
	tally collectionTally
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
		col.runningSteps = slices.DeleteFunc(col.runningSteps, func(s *taskState) bool { return s.state != Running })
		if len(col.runningSteps) > 0 {
			o.recordMisuse(ErrConcurrentRunning)
		}
		col.runningSteps = append(col.runningSteps, st)
	}
	st.state = Running
	o.armPlainHeartbeatLocked(st, o.cfg.clock.Now())
	// Every promoteRunningLocked call site already guards on st.state ==
	// Pending before calling it, and this line immediately advances past
	// Pending — so task.started fires exactly once per task's lifetime.
	o.emitWireEventLocked(wire.EventTaskStarted, st.id, nil)
}

func (o *Output) ensureOpen() error {
	if o.closed || o.finishing || o.finished {
		return ErrClosed
	}
	return nil
}

func (o *Output) bumpLocked() {
	o.version++
}

// Close is idempotent cleanup; best-effort Finish when needed.
func (o *Output) Close() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	needFinish := !o.finished
	o.mu.Unlock()
	if needFinish {
		_ = o.Finish()
	}
	o.mu.Lock()
	o.stopSpinnerAnimatorLocked()
	o.stopResizeWatchLocked()
	o.closed = true
	cancelRun := o.cancelRun
	manifestStore := o.manifestStore
	o.mu.Unlock()
	if cancelRun != nil {
		cancelRun()
	}
	// Finish already wrote the manifest and warned when it could not; this
	// releases the Run's exclusive manifest lock (spec §11.3) and returns
	// any write or release failure. Already committed Task records on disk
	// are unaffected — Close never rolls anything back.
	return manifestStore.Close()
}

// beginRunContext installs ctx (Run/evo.Run's own ctx parameter) as the
// parent of this run's task scopes, replacing the context.Background()
// Init installed as a placeholder for Define/Verify calls made before any
// Run. Every taskScopeHandle context (see withTaskScope) descends from
// o.Context(), so without this a caller's Run(ctx, ...) cancellation or
// deadline never reached a running Define/Verify — only the Init-time
// background context did (task scopes only ever observed SIGINT/Close via
// cancelRun, never the caller's own ctx or deadline).
//
// The previous run context's cancel is invoked here, not left to leak:
// nothing after this point should still be watching it, and a second Run
// call — on an Output whose caller reuses it after Close resets fields, or
// in a test exercising the mechanism directly — must start every new task
// scope from a fresh, uncancelled context rather than one inheriting a
// prior run's cancellation. Isolated outputs each hold their own o.ctx, so
// this never crosses between them.
func (o *Output) beginRunContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	previousCancel := o.cancelRun
	o.ctx = runCtx
	o.cancelRun = cancel
	o.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
	return runCtx
}

// Context reports the run's cancellation signal. It is cancelled when the
// run is interrupted (SIGINT/SIGTERM) and when the Output closes, so work
// that does I/O can select on it and stop instead of running on past the ^C
// that was supposed to end it. A nil Output reports a never-cancelled
// context so a caller never has to nil-check before selecting.
func (o *Output) Context() context.Context {
	if o == nil {
		return context.Background()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ctx == nil {
		return context.Background()
	}
	return o.ctx
}

// Conclusion returns the computed conclusion after Finish.
func (o *Output) Conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.conclusion != nil {
		return *o.conclusion
	}
	snap := o.snapshotLocked()
	c := core.InferConclusion(snap)
	o.explainCancellationLocked(&c)
	core.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
	return c
}

// cancelCauseUser is the cause an interrupt signal records: the person at
// the terminal stopped the run.
const cancelCauseUser = "by user"

// explainCancellationLocked names the cancellation cause on a cancelled
// conclusion. Any other outcome keeps its own Explanation untouched.
func (o *Output) explainCancellationLocked(c *core.Conclusion) {
	if c.State == core.StateCancelled && c.Explanation == "" {
		c.Explanation = o.cancelCause
	}
}
