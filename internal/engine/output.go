package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
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
	// use by File (workspaceDirLocked in file.go) so relative paths resolve
	// consistently even if the process CWD changes mid-Run (§8.1).
	workspaceDir string

	tasks       []*taskState
	collections []*tasksState
	changes     []*changesState
	plans       []*planState
	lines       []string
	actions     []Action
	events      []Event

	taskByRef  map[string]*taskState
	tasksByRef map[string]*tasksState
	keys       map[string]struct{}

	// namedTasks records every (scope, name) pair already declared through
	// Output.Task/Scope.Task (and, through them, the package-level
	// default-instance facade), so a repeated declaration under the same
	// pair is recognized as a duplicate sibling name (§3.1) instead of
	// silently merging two distinct declarations into one identity — 1.0
	// removed get-or-create for exactly that soundness reason (a merged
	// identity could later report a false "already satisfied"). Keyed
	// either "\x00"+scope+"\x00"+name (no explicit key) or "key:"+key
	// (explicit evo.ID); see taskScoped and taskNameByKey.
	namedTasks map[string]*TaskHandle
	// taskNameByKey remembers which display name first claimed an explicit
	// evo.ID through taskScoped: a second call reusing the same ID under any
	// name — same or different — is a real identity conflict (ErrDuplicateKey),
	// never a repeat declaration to be merged.
	taskNameByKey map[string]string
	// namedPlans/namedChanges back get-or-create identity for TaskHandle
	// mutation verbs (Delete, Create, ...): repeated mutations on one task
	// accumulate into the one Plan/Changes section named after the task,
	// instead of one section per call. Unrelated to Task/Group/Sequence
	// declaration identity (§3.1) — a ledger section is not a sibling.
	namedPlans   map[string]*planLedger
	namedChanges map[string]*changeLedger
	// namedReasons backs get-or-create identity for evo.Reason: repeated calls
	// with the same name (inline or lifted to a var) merge into one bucket.
	// Also unrelated to §3.1 — a taxonomy Reason is not a declared entity.
	namedReasons map[string]TaxonomyReason
	// namedGroups records every name already declared through evo.Sequence,
	// so a repeat is recognized as a duplicate sibling name (§3.1).
	namedGroups map[string]*SequenceHandle
	// namedGroupHandles records every name already declared through
	// evo.Group, so a repeat is recognized as a duplicate sibling name (§3.1).
	namedGroupHandles map[string]*GroupHandle

	// ctx is the run's own cancellation signal — the thing a callback doing
	// I/O selects on. cancelRun trips it on interrupt and on Close, so no
	// callback can outlive the run that owns it.
	ctx       context.Context
	cancelRun context.CancelFunc
	// schedCancelled stops the scheduler dispatching anything new: after an
	// interrupt the queue is abandoned, not drained.
	schedCancelled bool

	schedWG          sync.WaitGroup
	schedInflight    int
	schedMaxObserved int
	schedStartOrder  []string
	schedDraining    bool

	// schedExecuting counts task callbacks currently running, pooled and
	// donated alike — schedInflight counts only the pooled slots, so it
	// cannot answer "is any callback still moving?".
	schedExecuting int
	// schedWaits holds one ticket per goroutine parked in TaskHandle.Wait.
	// Together with schedExecuting it decides whether the run can still
	// progress, and it is how a wait that never can be satisfied is
	// released instead of hanging Finish (see releaseUnsatisfiableWaits).
	schedWaits map[*waitTicket]struct{}

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

	// activityAt is the domain-clock time of the most recent Phase, Progress,
	// or mutation-verb-callback-starting call — kept for the public
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

	// heartbeatRunningAt is the domain-clock time this task was promoted to
	// Running (see armPlainHeartbeatLocked in plain_heartbeat.go) — the §40
	// plain-mode durable heartbeat's elapsed anchor, mirroring liveFirstSeenAt's
	// role for the live renderer's elapsed suffix. Zero means no heartbeat is
	// armed for this task (interactive live presentation, or a TimeSource
	// that cannot schedule).
	heartbeatRunningAt time.Time
	// heartbeatDue is when the next plain-mode heartbeat check should
	// actually emit a line, pushed forward by any real durable emission
	// (deferPlainHeartbeatLocked) so a task that is genuinely narrating its
	// own progress never also gets a redundant heartbeat row.
	heartbeatDue time.Time

	// capture is the get-or-create sink shared by Task.Capture and PhaseWriter
	// so child-process evidence recorded via either path lands in one ring and
	// DetailTail sees it after Fail.
	evidence *evidence

	// skipped/kept hold disposition taxonomy accumulated by Skipped/Kept —
	// the model that "! skipped N (...)" / "! kept N (...)" are derived from
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

	fromEach    bool
	submitted   bool
	runningWork bool
	workFn      func() error
	mutation    *mutationSpec
	// effectDenied records that this task's own mutation callback resolved
	// the row as something other than Done, so the effect it was given must
	// not reach the ledger (see deniesItsOwnEffect).
	effectDenied bool
	preds        []predecessor
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

	// Plain/non-interactive progressive-streaming bookkeeping for a still-
	// Running standalone task (P10: CI logs must not stay silent until
	// Finish; beginner-8: a durable line per progress increment, thinned to
	// milestones for large totals). plainPhaseEmitted is the last Phase text
	// already streamed, so a repeated/no-op Phase call does not re-emit.
	// plainProgressStarted is false until the first Progress/Bytes tick;
	// plainProgressEmitted holds the last completed value actually streamed,
	// so a later tick knows whether it crossed a milestone boundary.
	plainPhaseEmitted    string
	plainProgressStarted bool
	plainProgressEmitted int64

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
	// declaration (see declareContainerLocked/declareChildContainerLocked).
	key         string
	name        string
	summary     string
	tasks       []*taskState
	declaration int
	handle      *GroupHandle

	// namedTasks records every name already declared as a child of this
	// container through Group.Task/Sequence.Task, so a repeated name is
	// recognized as a duplicate sibling (§3.1) instead of merging two
	// distinct declarations into one identity.
	namedTasks map[string]*TaskHandle

	// sequential marks a Sequence: children are chained in declaration
	// order. A Group's children are independent and may overlap.
	sequential bool

	// children holds nested containers declared via Sequence.Sequence,
	// Sequence.DisplayGroup, DisplayGroup.Sequence, or
	// DisplayGroup.DisplayGroup (P3's "both offer .Task/.Sequence/
	// .DisplayGroup, recursive") — a container's derived state and
	// rendering fold its children in exactly the way it folds its own
	// tasks.
	children []*tasksState

	// namedChildren records every name already declared as a nested
	// Group/Sequence child of this container, mirroring Output.namedGroups
	// but scoped to this container (container path + name is the identity,
	// so the same label under a different parent is a distinct child). A
	// repeated name here is a duplicate sibling (§3.1), not a get-or-create.
	namedChildren map[string]*tasksState
}

type changesState struct {
	id      string
	subject string
	records []EffectRecord
	// intendedVerb is the first mutation verb recorded for this section
	// (evo-rec.md "empty effect section grammar"). Set once, by
	// changes.go's Record/RecordName; it is what lets a section that ends up
	// with zero rows still render "nothing to <verb> <subject>" instead of a
	// generic fallback.
	intendedVerb string
	handle       *changeLedger
	// namedRowsEmitted is true once commitNamedEffectsLocked has already
	// streamed this section's rows durably at its owning task's resolution
	// (progressive.go) — Finish's residual ledger loop skips a section this
	// is true for so a named/enumerate section's items never render twice
	// (once live, once again at Finish).
	namedRowsEmitted bool
}

type planState struct {
	id      string
	subject string
	records []EffectRecord
	// intendedVerb mirrors changesState.intendedVerb for plan sections.
	intendedVerb string
	handle       *planLedger
	// namedRowsEmitted mirrors changesState.namedRowsEmitted for plan
	// sections.
	namedRowsEmitted bool
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

func (o *Output) nextID(prefix string) string {
	n := atomic.AddUint64(&o.idSeq, 1)
	return fmt.Sprintf("%s_%d", prefix, n)
}

func (o *Output) nextDecl() int {
	o.declSeq++
	return o.declSeq
}

func (o *Output) recordMisuse(err error) {
	if err == nil {
		return
	}
	if o.misuse == nil {
		o.misuse = err
	}
	if o.cfg.strict {
		panic(err)
	}
}

// hasRecordedEffectLocked reports whether subject already carries at least
// one mutation-ledger record (Changes or Plan) — see the unresolved-task
// auto-Done rescue in Finish (beginner-1, I1).
func (o *Output) hasRecordedEffectLocked(subject string) bool {
	for _, ch := range o.changes {
		if ch.subject == subject && len(ch.records) > 0 {
			return true
		}
	}
	for _, p := range o.plans {
		if p.subject == subject && len(p.records) > 0 {
			return true
		}
	}
	return false
}

// hasSealedProgress reports whether t's absolute progress reached the total
// it declared — a completed Each/EachN/Progress loop — same unresolved-task
// amnesty rationale as hasRecordedEffectLocked (beginner-gate-2 findings
// 1/2). Total must be positive and the kind explicitly set (Determinate or
// BytesKind): a task that never called Progress/Bytes/Each carries the zero
// value (Total 0, Kind "") and must not read as sealed.
func hasSealedProgress(t *taskState) bool {
	if t.progress.Kind == "" || t.progress.Kind == Indeterminate {
		return false
	}
	return t.progress.Total > 0 && t.progress.Completed >= t.progress.Total
}

// hasRecordedTaxonomy reports whether t accumulated any Skipped/Kept record
// — same unresolved-task amnesty rationale as hasRecordedEffectLocked
// (beginner-gate-2 finding 4): the disposition taxonomy already told an
// honest, complete story even though nothing called a terminal verb.
func hasRecordedTaxonomy(t *taskState) bool {
	return len(t.skipped) > 0 || len(t.kept) > 0
}

// appendMisuseLineLocked renders the one required line for the first
// recorded misuse — an exit code may never disagree with everything the
// caller saw printed (beginner-1). The line is misuseHintFor's corrective
// sentence for the recorded sentinel, never the raw "evo: ..." sentinel text
// (release-gate round 4 finding 2): machine detail stays in JSON/debug, the
// human stream gets told what to do next.
func (o *Output) appendMisuseLineLocked() {
	hint := misuseHintFor(o.misuse, o.misuseSubject, o.misuseRejectedSummary)
	glyph := txt.StyleGlyph(misuseGlyph, txt.SGRYellow, !o.cfg.noColor)
	o.lines = append(o.lines, fmt.Sprintf("%s  %s", glyph, hint))
}

// recordMisuseFor is recordMisuse with the offending entity's name attached,
// so Finish can name it in the one required misuse line (beginner-1) instead
// of an exit code silently disagreeing with everything the caller saw
// rendered.
func (o *Output) recordMisuseFor(subject string, err error) {
	if err == nil {
		return
	}
	if o.misuse == nil {
		o.misuseSubject = subject
	}
	o.recordMisuse(err)
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Done/Fail/Block/Warn/Cancel/Skip) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Output) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	if o.misuse == nil {
		o.misuseRejectedSummary = rejectedSummary
	}
	o.recordMisuseFor(name, ErrAlreadyResolved)
}

// promoteRunningLocked transitions a Pending task to Running on its first
// unit of evidence (Phase/Progress/Advance/Bytes/Each iteration/PhaseWriter
// write, or a mutation-verb callback starting — see promoteRunningForActivity).
// For a sequential collection (Sequence), it records misuse when a sibling is
// already Running, enforcing the heart contract "one Running child"
// (evo-rec.md) — callers still get the transition; Strict mode is what
// escalates the violation to a panic. A plain DisplayGroup collection
// documents its children as independent (worker-pool fan-out is a
// supported, concurrency-safe pattern there), so it is not policed.
func (o *Output) promoteRunningLocked(st *taskState) {
	if st.collection != nil && st.collection.sequential {
		for _, sibling := range st.collection.tasks {
			if sibling != st && sibling.state == Running {
				o.recordMisuse(ErrConcurrentRunning)
				break
			}
		}
	}
	st.state = Running
	o.armPlainHeartbeatLocked(st, o.cfg.clock.Now())
}

func (o *Output) ensureOpen() error {
	if o.closed || o.finishing || o.finished {
		return ErrClosed
	}
	return nil
}

// Err returns the first recorded misuse error, if any.
func (o *Output) Err() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.misuse
}

func (o *Output) ensureEntityRoomLocked() error {
	n := len(o.tasks)
	if n >= o.cfg.maxEntities {
		return ErrLimitExceeded
	}
	return nil
}

// Task declares a single operation. Optional evo.ID sets a stable machine
// key. name is a printf format when args are present (fmt.Sprintf
// semantics) — evo.ID (or any other EntityOption) may be mixed into args in
// any position and still applies.
func (o *Output) Task(name string) *TaskHandle {
	return o.taskScoped(name, "")
}

// taskScoped is the declaration path behind Output.Task and Scope.Task. A
// repeated call with the same (scope, name) pair — or a repeated explicit
// evo.ID under any name — is a duplicate sibling declaration (§3.1), never a
// get-or-create: 1.0 removed that idiom because letting two distinct
// declarations silently merge into one identity would make a false
// "already satisfied" possible once identity drives manifest reconciliation.
// A same-name repeat records a Failed task with ProblemCodeDuplicateSiblingName
// (see failDuplicateSiblingLocked); a reused explicit key still reports
// ErrDuplicateKey, its own pre-existing identity-conflict error.
func (o *Output) taskScoped(name, scope string, opts ...EntityOption) *TaskHandle {
	eo := applyEntityOptions(opts)
	clean := declaredName(name)
	key := qualifyKey(scope, eo.key)

	o.mu.Lock()
	defer o.mu.Unlock()

	if key != "" {
		if _, ok := o.taskNameByKey[key]; ok {
			o.recordMisuse(ErrDuplicateKey)
			return &TaskHandle{out: o, id: o.nextID("task")}
		}
	} else if _, ok := o.namedTasks["\x00"+scope+"\x00"+clean]; ok {
		o.failDuplicateSiblingLocked(nil, kindTask, clean)
		return &TaskHandle{out: o, id: o.nextID("task")}
	}

	h := o.addTaskLocked(clean, nil, key, scope, false)
	if o.namedTasks == nil {
		o.namedTasks = make(map[string]*TaskHandle)
	}
	if key != "" {
		if o.taskNameByKey == nil {
			o.taskNameByKey = make(map[string]string)
		}
		o.taskNameByKey[key] = clean
		o.namedTasks["key:"+key] = h
	} else {
		o.namedTasks["\x00"+scope+"\x00"+clean] = h
	}
	if eo.phase != "" {
		if st := o.taskByRef[h.id]; st != nil && o.ensureOpen() == nil && !core.IsTerminalTask(st.state) {
			o.setPhaseLocked(st, eo.phase)
		}
	}
	return h
}

// declaredTaskState is Pending until the scheduler starts the Task.
func declaredTaskState(col *tasksState) EntityState {
	_ = col
	return Pending
}

// ledgerSubjectFor names the subject an effect belongs to. An explicitly
// declared task is semantically named work and owns its own ledger line. An
// Each child does not: it is one item of a collection, and the collection is
// the subject the plan is about. Cleaning 33 branches recorded 33 separate
// `[planned] feat/old-03  delete 1 local tip` rows, one per item name and
// unbounded, where the dialect's own Recommended UI for that run shows one:
// `[planned] branches  delete 33 local tips`.
//
// Attributing at the record site rather than folding rows in the renderer is
// what makes the rest fall out: the existing identical-record merge does the
// tally, and a caller that does want item names gets them through RecordName
// under the collection's subject, inside the same bounded viewport and
// `… +N more (not shown)` overflow every other subject has.
func ledgerSubjectFor(st *taskState) string {
	if st.fromEach && st.collection != nil {
		return st.collection.name
	}
	return st.name
}

func (o *Output) addTaskLocked(name string, col *tasksState, key, parentKey string, fromEach bool) *TaskHandle {
	h := o.declareTaskLocked(name, col, key, parentKey, fromEach)
	if _, ok := o.taskByRef[h.id]; ok {
		o.signalLiveLocked(true)
	}
	return h
}

// declareTaskLocked records a child without painting. Each uses this to
// declare every item before the first yield so the first live frame already
// shows 0/N rather than growing 0/1, 0/2, … as children appear.
//
// When key is empty, the task's §3.1 stable identity defaults to
// kind+parentKey+normalized-name; an explicit key replaces that derivation
// entirely and is registered instead. parentKey is the declaring parent's
// own stable key (a Group/Sequence's key, or the declaration scope for a
// root-level Task — see Scope).
func (o *Output) declareTaskLocked(name string, col *tasksState, key, parentKey string, fromEach bool) *TaskHandle {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	effectiveKey := key
	if effectiveKey != "" {
		if _, ok := o.keys[effectiveKey]; ok {
			o.recordMisuse(ErrDuplicateKey)
			return &TaskHandle{out: o, id: o.nextID("task")}
		}
		o.keys[effectiveKey] = struct{}{}
	} else {
		effectiveKey = stableKey(kindTask, parentKey, name)
	}
	if err := o.ensureEntityRoomLocked(); err != nil {
		o.recordMisuse(err)
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	st := &taskState{
		id:          o.nextID("task"),
		key:         effectiveKey,
		name:        name,
		state:       declaredTaskState(col),
		progress:    Progress{Kind: Indeterminate},
		collection:  col,
		declaration: o.nextDecl(),
		doneCh:      make(chan struct{}),
		fromEach:    fromEach,
		resolution:  ResolutionNoWork,
	}
	h := &TaskHandle{out: o, id: st.id}
	st.handle = h
	o.tasks = append(o.tasks, st)
	if col != nil {
		col.tasks = append(col.tasks, st)
	}
	o.taskByRef[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.declared", EntityID: st.id})
	return h
}

// planGetOrCreate returns the Plan previously created under subject by this
// method, or declares a new one — the identity backing TaskHandle mutation
// verbs, where repeated dry-run mutations on one task accumulate into one
// [planned] section instead of a new one per call.
func (o *Output) planGetOrCreate(subject string) *planLedger {
	o.mu.Lock()
	defer o.mu.Unlock()
	if p, ok := o.namedPlans[subject]; ok {
		return p
	}
	p := o.declarePlanLedgerLocked(subject)
	if o.namedPlans == nil {
		o.namedPlans = make(map[string]*planLedger)
	}
	o.namedPlans[subject] = p
	return p
}

// changesGetOrCreate is planGetOrCreate's counterpart for applied mutations.
func (o *Output) changesGetOrCreate(subject string) *changeLedger {
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok := o.namedChanges[subject]; ok {
		return c
	}
	c := o.declareChangeLedgerLocked(subject)
	if o.namedChanges == nil {
		o.namedChanges = make(map[string]*changeLedger)
	}
	o.namedChanges[subject] = c
	return c
}

// cancelActive cancels the currently running task, or the output itself when
// no task is running, so an interrupt always leaves a typed Cancelled state.
//
// A pending Confirm gate takes priority over the generic task scan below: a
// gate holds sole control of the run (Confirm suspends the live region and
// blocks on stdin) and its abort channel — not TaskHandle.Cancel — is what
// unblocks the stdin read. Since a Confirm gate is an ordinary Task while
// its answer is pending, the generic Pending-task fallback would otherwise
// resolve it to Cancelled without ever closing that channel, leaving
// readConfirmLine blocked forever.
// interrupt stops the run at the first signal, in the one order that leaves
// the ledger honest: the scheduler is closed to new work, every row is put
// into the state the reader must see, and only then is the run's context
// cancelled to release the callbacks still in flight. Cancelling first would
// race a finishing callback into a ✓ row after the ^C.
func (o *Output) interrupt(reason string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.schedCancelled = true
	cancelRun := o.cancelRun
	o.mu.Unlock()

	o.cancelActive(reason)
	o.abandonQueuedWork()

	if cancelRun != nil {
		cancelRun()
	}
}

// abandonQueuedWork resolves every task that had not begun as NotStarted —
// the interrupt's answer to "and what about the rest?", which the reader
// would otherwise never get.
//
// Submitted or not: a task the caller declared and never Defined is work the
// interrupt took away just as surely as one sitting in the scheduler's
// queue. Sweeping only the submitted ones left a declared row Pending, and
// Finish then charged the caller with ErrUnresolvedTask and told them to
// "call Done, Fail, Block, Skipped, or a mutation verb on this task" about a
// run the user had just cancelled.
func (o *Output) abandonQueuedWork() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, st := range o.tasks {
		if st.runningWork || core.IsTerminalTask(st.state) {
			continue
		}
		o.markNotStartedLocked(st)
	}
}

func (o *Output) cancelActive(reason string) {
	o.mu.Lock()
	if o.cancelPendingConfirmLocked(reason) {
		o.mu.Unlock()
		return
	}
	running := make([]*TaskHandle, 0, len(o.tasks))
	for _, t := range o.tasks {
		if t.state == Running {
			running = append(running, t.handle)
		}
	}
	if len(running) > 0 {
		o.mu.Unlock()
		for _, t := range running {
			t.Cancel(reason)
		}
		return
	}
	// Nothing has reached Running yet: the earliest-declared Pending task is
	// the one about to run next (evo-rec.md "one Running child" — pending
	// siblings are named and idle, waiting their turn), so an interrupt
	// before any evidence still cancels that task rather than falling
	// through to Output-level cancel.
	var active *TaskHandle
	for _, t := range o.tasks {
		if t.state == Pending {
			active = t.handle
			break
		}
	}
	if active != nil {
		o.mu.Unlock()
		active.Cancel(reason)
		return
	}
	o.mu.Unlock()
	o.Cancel(reason)
}

// cancelPendingConfirmLocked cancels one pending Confirm gate, if any, so ^C
// at a "[y/N]" prompt unblocks Confirm's stdin read and resolves the gate as
// Cancelled — never Blocked "declined" (a human "n" and an interrupt are
// distinct outcomes). Reports whether a gate was cancelled.
func (o *Output) cancelPendingConfirmLocked(reason string) bool {
	for id, abort := range o.confirmAbort {
		close(abort)
		delete(o.confirmAbort, id)
		if st := o.taskByRef[id]; st != nil && !core.IsTerminalTask(st.state) {
			st.state = Cancelled
			st.summary = txt.Text(reason)
			o.bumpLocked()
			o.appendEventLocked(Event{Type: "task.cancelled", EntityID: id})
			o.commitResolvedTaskLocked(id)
		}
		return true
	}
	return false
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create — see failDuplicateSiblingLocked.
// name is a printf format when args are present.
func (o *Output) Group(name string) *GroupHandle {
	clean := declaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.namedGroupHandles[clean]; ok {
		o.failDuplicateSiblingLocked(nil, kindGroup, clean)
		return &GroupHandle{out: o, id: o.nextID("tasks")}
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &GroupHandle{out: o, id: o.nextID("tasks")}
	}
	st := o.declareContainerLocked(clean, false)
	o.collections = append(o.collections, st)
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	if o.namedGroupHandles == nil {
		o.namedGroupHandles = make(map[string]*GroupHandle)
	}
	o.namedGroupHandles[clean] = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	return h
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated
// evo.Sequence("python") call is a duplicate sibling declaration (§3.1), not
// a get-or-create — see failDuplicateSiblingLocked. name is a printf format
// when args are present (fmt.Sprintf semantics); no args leaves name
// untouched.
func (o *Output) Sequence(name string) *SequenceHandle {
	clean := declaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.namedGroups[clean]; ok {
		o.failDuplicateSiblingLocked(nil, kindSequence, clean)
		return &SequenceHandle{tasks: &GroupHandle{out: o, id: o.nextID("tasks")}}
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &SequenceHandle{tasks: &GroupHandle{out: o, id: o.nextID("tasks")}}
	}
	st := o.declareContainerLocked(clean, true)
	o.collections = append(o.collections, st)
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	g := &SequenceHandle{tasks: h}
	if o.namedGroups == nil {
		o.namedGroups = make(map[string]*SequenceHandle)
	}
	o.namedGroups[clean] = g
	return g
}

// declareContainerLocked allocates a new top-level tasksState — the shared
// body behind Group and Sequence, which differ only in the sequential flag.
// name must already be declaredName-normalized — Group and Sequence
// normalize once at declaration entry, before their sibling-dedup check.
func (o *Output) declareContainerLocked(name string, sequential bool) *tasksState {
	st := &tasksState{
		id:          o.nextID("tasks"),
		key:         stableKey(childKindFor(sequential), "", name),
		name:        name,
		declaration: o.nextDecl(),
		sequential:  sequential,
	}
	o.tasksByRef[st.id] = st
	return st
}

// childKindFor names the entity kind a nested container declares, for the
// duplicate-sibling problem it may need to report.
func childKindFor(sequential bool) entityKind {
	if sequential {
		return kindSequence
	}
	return kindGroup
}

// declareChildContainerLocked declares a nested container under parent,
// scoped to this one parent (path + name is the identity). A repeated name
// is a duplicate sibling declaration (§3.1); the caller (GroupHandle/
// SequenceHandle.Group/Sequence) still receives a usable, if orphaned,
// handle back.
func (o *Output) declareChildContainerLocked(parent *tasksState, name string, sequential bool) *tasksState {
	clean := declaredName(name)
	kind := childKindFor(sequential)
	if _, ok := parent.namedChildren[clean]; ok {
		o.failDuplicateSiblingLocked(parent, kind, clean)
		return &tasksState{id: o.nextID("tasks"), name: clean, sequential: sequential}
	}
	st := &tasksState{
		id:          o.nextID("tasks"),
		key:         stableKey(kind, parent.key, clean),
		name:        clean,
		declaration: o.nextDecl(),
		sequential:  sequential,
	}
	o.tasksByRef[st.id] = st
	parent.children = append(parent.children, st)
	if parent.namedChildren == nil {
		parent.namedChildren = make(map[string]*tasksState)
	}
	parent.namedChildren[clean] = st
	return st
}

// declareGroupTask declares a child task by name in the container backed by
// groupID — the identity behind Group.Task/Sequence.Task. A repeated name is
// a duplicate sibling declaration (§3.1), not a get-or-create.
func (o *Output) declareGroupTask(groupID, name string, opts ...EntityOption) *TaskHandle {
	clean := declaredName(name)
	o.mu.Lock()
	col := o.tasksByRef[groupID]
	if col == nil {
		o.mu.Unlock()
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	if _, ok := col.namedTasks[clean]; ok {
		o.failDuplicateSiblingLocked(col, kindTask, clean)
		o.mu.Unlock()
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	eo := applyEntityOptions(opts)
	h := o.addTaskLocked(clean, col, eo.key, col.key, false)
	if col.namedTasks == nil {
		col.namedTasks = make(map[string]*TaskHandle)
	}
	col.namedTasks[clean] = h
	o.mu.Unlock()
	return h
}

// declareChangeLedgerLocked starts a durable-effects section named subject —
// the internal counterpart of the deleted public Output.Changes entry point
// (P1/P13: presentation-decision aPI, callers reach effects only through
// TaskHandle's mutation verbs now). Caller must hold o.mu.
func (o *Output) declareChangeLedgerLocked(subject string) *changeLedger {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &changeLedger{out: o, id: o.nextID("changes")}
	}
	st := &changesState{
		id:      o.nextID("changes"),
		subject: txt.Text(subject),
	}
	h := &changeLedger{out: o, id: st.id}
	st.handle = h
	o.changes = append(o.changes, st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "changes.declared", EntityID: st.id})
	return h
}

// declarePlanLedgerLocked starts a would-occur effects section named
// subject — the internal counterpart of the deleted public Output.Plan
// entry point (see declareChangeLedgerLocked). Caller must hold o.mu.
func (o *Output) declarePlanLedgerLocked(subject string) *planLedger {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &planLedger{out: o, id: o.nextID("plan")}
	}
	st := &planState{
		id:      o.nextID("plan"),
		subject: txt.Text(subject),
	}
	h := &planLedger{out: o, id: st.id}
	st.handle = h
	o.plans = append(o.plans, st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "plan.declared", EntityID: st.id})
	return h
}

// Fail records an output-level failure.
func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.failWith(applyProblemOptions(txt.Text(summary), options))
}

// Failf records an output-level failure with a formatted summary. fmt.Errorf
// semantics: a trailing ": %w"/", %w" splits the formatted text into the
// recorded summary and evidence line exactly like TaskHandle.Failf.
//
// Failf stays void rather than returning an error like TaskHandle.Failf does
// (release-gate round 4 finding 5): every existing call site uses Failf as a
// bare statement (e.g. Output.Run's own runInterruptible), and errcheck
// flags a discarded error return with no lint-config exception on this repo
// — so matching TaskHandle.Failf's signature here would force every one of
// those call sites to add a needless `_ = ` just to stay lint-clean. There is
// also no per-call Next chain to attach an error return to here the way
// TaskHandle.Failf's *Failure does (Output.Next already covers the
// output-level case), so a returned error would carry less than
// TaskHandle.Failf's does anyway. Documented asymmetry, not an oversight.
func (o *Output) Failf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	summary, evidence := core.SplitWrappedMessage(format, err)
	o.failWith(core.SanitizeProblem(Problem{Summary: summary, Detail: evidence}))
}

func (o *Output) failWith(p Problem) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	// Synthetic failed task for conclusion.
	st := &taskState{
		id:          o.nextID("task"),
		name:        txt.Text(o.cfg.subject),
		state:       Failed,
		problems:    []Problem{p},
		declaration: o.nextDecl(),
		synthetic:   true,
	}
	if st.name == "" {
		st.name = identityFallbackName()
	}
	o.tasks = append(o.tasks, st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.failed"})
}

// Cancel records output-level cancellation via a synthetic cancelled task.
func (o *Output) Cancel(reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	name := txt.Text(o.cfg.subject)
	if name == "" {
		name = identityFallbackName()
	}
	t := &taskState{
		id:          o.nextID("task"),
		name:        name,
		state:       Cancelled,
		summary:     txt.Text(reason),
		declaration: o.nextDecl(),
		synthetic:   true,
	}
	o.tasks = append(o.tasks, t)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.cancelled"})
}

// Next attaches output-level actions.
func (o *Output) Next(actions ...Action) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.actions = append(o.actions, cloneActions(actions)...)
	o.bumpLocked()
}

// NextCommand attaches an output-level command action.
func (o *Output) NextCommand(executable string, args ...string) {
	o.Next(Command(executable, args...))
}

// Debug records a structured diagnostic (§4.6 / §21.3).
//
// History mode (default): durable scrollback above the live region (or plain stream).
// Pane mode: record is journaled and shown in the rolling live pane; not durable
// scrollback unless a diagnostic tail is preserved at Finish.
//
// When Diagnostics is configured and is a different writer than the primary stream,
// debug lines go to Diagnostics only (not the human Items/Tasks stream). Use
// Capture for child-process evidence instead of DebugWriter when you need Fail Detail.
func (o *Output) debug(message string, fields ...Field) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitDebugLocked(message, fields, false)
}

func (o *Output) emitDebugLocked(message string, fields []Field, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > LevelDebug {
		return
	}
	rec := o.newDebugRecordLocked("DEBUG", message, fields, time.Time{})
	o.projectDebugRecordLocked(rec)
}

// emitDebugRecordLocked journals a fully-formed debug record (slog bridge).
// force bypasses DebugLevel filtering after slog already applied its min level.
func (o *Output) emitDebugRecordLocked(levelName, message string, fields []Field, at time.Time, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > LevelDebug {
		return
	}
	rec := o.newDebugRecordLocked(levelName, message, fields, at)
	o.projectDebugRecordLocked(rec)
}

func (o *Output) newDebugRecordLocked(levelName, message string, fields []Field, at time.Time) debugRecord {
	if at.IsZero() {
		at = o.cfg.clock.Now()
	}
	rec := debugRecord{
		Time:    at,
		Level:   levelName,
		Message: txt.Text(message),
		Fields:  cloneFields(fields),
	}
	for i := range rec.Fields {
		if rec.Fields[i].Sensitive {
			rec.Fields[i].Value = "***"
		} else if o.cfg.redactor != nil {
			rec.Fields[i].Value = o.cfg.redactor.RedactString(fmt.Sprint(rec.Fields[i].Value))
		}
	}
	return rec
}

func (o *Output) projectDebugRecordLocked(rec debugRecord) {
	o.debugRecords = append(o.debugRecords, rec)
	history := formatHistoryLine(rec, !o.cfg.noColor)
	plainHistory := formatHistoryLine(rec, false)
	o.lines = append(o.lines, plainHistory)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "log.emitted"})

	dual := o.cfg.diagnostic != nil && o.cfg.primary != nil && o.cfg.diagnostic != o.cfg.primary
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain
	panePresentation := o.cfg.debugPresentation == DebugPresentationPane

	// dualSameTerminal is true when Diagnostics is a distinct writer from
	// primary but both resolve to the SAME physical terminal (the realistic
	// default: Config{Stdout, Stderr} on an interactive shell, where fd 1 and
	// fd 2 name one controlling tty). Writing raw bytes straight to
	// Diagnostics in that case bypasses the live region's clear-live ->
	// write-durable -> repaint sequencing and corrupts the spinner row on
	// the one screen both writers share — route it through the same
	// sequencing single-stream records use instead.
	dualSameTerminal := dual && interactive && o.cfg.diagnosticSharesTerminal

	if o.cfg.diagnostic != nil && !dualSameTerminal {
		o.writeDiagnosticTextLocked(plainHistory + "\n")
	}
	if dual && !dualSameTerminal {
		// The record already went to Diagnostics above; don't also duplicate
		// it onto the live pane/durable primary. Still mark pane presentation
		// as active so a failing Finish can preserve the diagnostics tail on
		// the live terminal (§21.3.2) — otherwise the promised failure tail
		// is unreachable whenever the caller uses two distinct real streams
		// (the realistic default: Config{Stdout, Stderr} routes to(Stdout),
		// withDiagnostics(Stderr)).
		if interactive && panePresentation {
			o.debugPaneActive = true
		}
		o.linesEmitted = len(o.lines)
		return
	}

	if interactive && panePresentation {
		o.debugPaneActive = true
		o.linesEmitted = len(o.lines)
		o.signalLiveLocked(true)
		return
	}
	if interactive {
		o.debugLiveLocked(history)
	} else {
		o.writeDurableTextLocked(history + "\n")
	}
	o.linesEmitted = len(o.lines)
}

// writeDiagnosticText emits text on the Diagnostics writer (thread-safe).
func (o *Output) writeDiagnosticText(text string) {
	if o == nil || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writeDiagnosticTextLocked(text)
}

func (o *Output) writeDiagnosticTextLocked(text string) {
	if o.cfg.diagnostic == nil || text == "" {
		return
	}
	_, _ = io.WriteString(o.cfg.diagnostic, text)
	if f, ok := o.cfg.diagnostic.(flusher); ok {
		_ = f.Flush()
	}
}

func cloneFields(in []Field) []Field {
	if len(in) == 0 {
		return nil
	}
	out := make([]Field, len(in))
	copy(out, in)
	return out
}

// Snapshot returns an immutable copy of current state.
func (o *Output) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshotLocked()
}

func (o *Output) snapshotLocked() Snapshot {
	s := Snapshot{
		Version:       o.version,
		OutputID:      o.outputID,
		Subject:       o.cfg.subject,
		Lines:         append([]string(nil), o.lines...),
		Actions:       cloneActions(o.collectActionsLocked()),
		Timestamp:     o.cfg.clock.Now(),
		DryRun:        o.cfg.dryRun,
		Preview:       o.cfg.preview,
		DryRunSubject: o.cfg.dryRunHeaderText,
		Warnings:      core.CloneProblems(o.runWarnings),
		Facts:         core.CloneFacts(o.runFacts),
	}
	for _, col := range o.collections {
		s.Collections = append(s.Collections, col.snapshot())
	}
	// Root tasks not in a collection
	for _, t := range o.tasks {
		if t.collection == nil {
			s.Tasks = append(s.Tasks, t.snapshot())
		}
	}
	for _, ch := range o.changes {
		s.Changes = append(s.Changes, ch.snapshot())
	}
	for _, p := range o.plans {
		s.Plans = append(s.Plans, p.snapshot())
	}
	for _, m := range o.messages {
		s.Messages = append(s.Messages, MessageSnapshot{
			ID:         m.id,
			Text:       m.text,
			Visibility: m.visibility,
		})
	}
	if o.conclusion != nil {
		c := *o.conclusion
		s.Conclusion = &c
	}
	return s
}

func (t *taskState) snapshot() TaskSnapshot {
	colID := ""
	if t.collection != nil {
		colID = t.collection.id
	}
	base := TaskSnapshot{
		ID:          t.id,
		Key:         t.key,
		Name:        t.name,
		State:       t.state,
		Phase:       t.phase,
		ActivityAt:  t.activityAt,
		Progress:    t.progress,
		Summary:     t.summary,
		Problems:    core.CloneProblems(t.problems),
		Warnings:    core.CloneProblems(t.warnings),
		Facts:       core.CloneFacts(t.facts),
		Actions:     cloneActions(t.actions),
		Skipped:     cloneTaxonomy(t.skipped),
		Kept:        cloneTaxonomy(t.kept),
		Collection:  colID,
		Declaration: t.declaration,
		Resolution:  t.resolution,
		Evidence:    t.verifyEvidence,
	}
	return core.NewTaskSnapshot(base, t.liveFirstSeenAt, t.synthetic, t.fromEach)
}

func cloneTaxonomy(in []TaxonomyRecord) []TaxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]TaxonomyRecord, len(in))
	copy(out, in)
	for i := range out {
		if len(out[i].Causes) > 0 {
			out[i].Causes = append([]string(nil), out[i].Causes...)
		}
	}
	return out
}

func (g *tasksState) snapshot() TasksSnapshot {
	ts := TasksSnapshot{
		ID:          g.id,
		Key:         g.key,
		Name:        g.name,
		State:       g.derivedState(),
		Summary:     g.displaySummary(),
		Declaration: g.declaration,
		Sequential:  g.sequential,
	}
	for _, t := range g.tasks {
		ts.Tasks = append(ts.Tasks, t.snapshot())
	}
	for _, child := range g.children {
		ts.Collections = append(ts.Collections, child.snapshot())
	}
	return ts
}

// derivedState folds both this container's own tasks and its nested
// children (P3's recursive nesting) into one verdict: a nested Sequence or
// DisplayGroup contributes exactly like one more task would, so a failure
// three levels deep still surfaces at the root header.
func (g *tasksState) derivedState() EntityState {
	if len(g.tasks) == 0 && len(g.children) == 0 {
		return Empty
	}
	var anyRunning, anyFailed, anyCancelled, anyUnresolved bool
	allDone := true
	// A NotStarted child normally borrows its group's verdict from the
	// sibling that failed first, so it contributes nothing of its own. When
	// every child is NotStarted there is no such sibling — whatever stopped
	// the run was another subject entirely — and folding to Done rendered a
	// check over a subject that never ran.
	allNotStarted := len(g.tasks) > 0 && len(g.children) == 0
	for _, t := range g.tasks {
		if t.state != NotStarted {
			allNotStarted = false
		}
		switch t.state {
		case Running:
			anyRunning = true
			allDone = false
		case Pending:
			anyUnresolved = true
			allDone = false
		case Failed:
			anyFailed = true
		case Cancelled:
			anyCancelled = true
		case Done, Skipped:
		case NotStarted:
		default:
			anyUnresolved = true
			allDone = false
		}
	}
	if allNotStarted {
		return NotStarted
	}
	for _, child := range g.children {
		switch child.derivedState() {
		case Running:
			anyRunning = true
			allDone = false
		case Failed:
			anyFailed = true
			allDone = false
		case Cancelled:
			anyCancelled = true
			allDone = false
		case Done, Empty:
		default:
			anyUnresolved = true
			allDone = false
		}
	}
	if anyRunning {
		return Running
	}
	if anyFailed {
		return Failed
	}
	if anyCancelled {
		return Cancelled
	}
	if anyUnresolved {
		return Incomplete
	}
	if allDone {
		return Done
	}
	return Incomplete
}

func (g *tasksState) displaySummary() string {
	// Success summary only when all children done/skipped successfully.
	st := g.derivedState()
	if st == Done && g.summary != "" && !g.hasWarnedOrFailedDescendant() {
		return g.summary
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
	for _, t := range g.tasks {
		if t.state == Failed || t.state == Cancelled || len(t.warnings) > 0 {
			return true
		}
	}
	for _, child := range g.children {
		if child.hasWarnedOrFailedDescendant() {
			return true
		}
	}
	return false
}

func (c *changesState) snapshot() ChangesSnapshot {
	return ChangesSnapshot{
		ID:           c.id,
		Subject:      c.subject,
		Records:      append([]EffectRecord(nil), c.records...),
		IntendedVerb: c.intendedVerb,
	}
}

func (p *planState) snapshot() PlanSnapshot {
	return PlanSnapshot{
		ID:           p.id,
		Subject:      p.subject,
		Records:      append([]EffectRecord(nil), p.records...),
		IntendedVerb: p.intendedVerb,
	}
}

func (o *Output) collectActionsLocked() []Action {
	seen := map[string]struct{}{}
	var out []Action
	add := func(list []Action) {
		for _, a := range list {
			k := actionKey(a)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, a)
		}
	}
	add(o.actions)
	for _, t := range o.tasks {
		add(t.actions)
	}
	return out
}

func (o *Output) bumpLocked() {
	o.version++
}

func (o *Output) appendEventLocked(e Event) {
	e.Timestamp = o.cfg.clock.Now()
	e.SchemaVersion = EventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = o.outputID
	}
	// Sequence is monotonic assignment order, not index after compaction.
	e.Sequence = uint64(len(o.events) + 1)
	if len(o.events) > 0 {
		e.Sequence = o.events[len(o.events)-1].Sequence + 1
	}
	o.events = append(o.events, e)
	o.compactJournalLocked()
	if o.cfg.projection == ProjectionStreamJSON {
		o.writeStreamJSONLocked(e)
	}
	if o.cfg.wireFormat == FormatJSONL {
		writeWireEventLocked(o.cfg.wireStream, e)
	}
}

func (o *Output) writeStreamJSONLocked(e Event) {
	w := o.cfg.primary
	if w == nil {
		return
	}
	row, err := render.EncodeEventJSON(e)
	if err != nil {
		return
	}
	_, _ = w.Write(row)
	_, _ = w.Write([]byte{'\n'})
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
}

// criticalEventTypes are never dropped under journal backpressure (CON-008).
func criticalEventType(t string) bool {
	switch t {
	case "output.failed", "output.finished", "output.cancelled",
		"task.blocked", "task.failed", "output.started":
		return true
	default:
		return false
	}
}

func (o *Output) compactJournalLocked() {
	max := o.cfg.maxEvents
	if max <= 0 || len(o.events) <= max {
		return
	}
	// Drop oldest non-critical until under cap; if still over, drop oldest critical last.
	for len(o.events) > max {
		drop := -1
		for i, ev := range o.events {
			if !criticalEventType(ev.Type) {
				drop = i
				break
			}
		}
		if drop < 0 {
			drop = 0
		}
		o.events = append(o.events[:drop], o.events[drop+1:]...)
	}
}

// notStartedSummary is the literal detail rendered for an auto-resolved group
// child — fixed text, not caller-composed, so every call site spells it the
// same way (evo-rec.md early-termination examples: "-  install  not started").
const notStartedSummary = "not started"

// unresolvedTaskCancelledSummary is the literal detail rendered for a plain
// (non-Group) task still Running when Finish is reached during an abnormal
// finish (a real SIGINT/SIGTERM cancellation or an application error already
// recorded elsewhere in the run) — the run concluded without a caller-
// recorded verdict AND something really did cut it short, so the honest
// terminal state is Cancelled rather than a stuck/incomplete glyph.
const unresolvedTaskCancelledSummary = "cancelled — run concluded before finish"

// unresolvedTaskIncompleteSummary is the literal detail rendered for a plain
// (non-Group) task still Running when Finish is reached during an otherwise
// clean finish — no signal, no application error anywhere in the run. Cancel
// (and its 130 exit) is reserved for a real interruption signal (release-gate
// finding 1); a caller who simply forgot a terminal verb gets an honest
// incomplete reading instead, folded into the conclusion as Partial rather
// than invented as a headline of its own.
const unresolvedTaskIncompleteSummary = "incomplete — run concluded before finish"

// resolveUnstartedTaskLocked derives a real terminal state for a task Finish
// found non-terminal during an abnormal finish (abnormalFinishLocked), from
// the one model, so every consumer sees the same honest verdict instead of
// hand-rolling the same cascade themselves: a task that had already started
// (Running) reads as Cancelled, and a task that never got attention
// (Pending) reads as NotStarted ("not started") — the same face a Group
// gives an unstarted sibling (autoResolveGroupsLocked). Never called for a
// task an explicit verb already resolved, and never called on a clean finish
// at all (see Finish's own !abnormal branch, which resolves every
// non-terminal task to Incomplete directly instead, regardless of whether it
// ever reached Running — release-gate round 4 finding 3).
func resolveUnstartedTaskLocked(t *taskState) {
	if t.state == Running {
		t.state = Cancelled
		t.phase = ""
		t.summary = unresolvedTaskCancelledSummary
		return
	}
	t.state = NotStarted
	t.phase = ""
	t.summary = notStartedSummary
}

// abnormalFinishLocked reports whether the run already carries a real Failed
// or Cancelled task by the time Finish's unresolved-task sweep runs —
// evidence that something genuinely interrupted the run (Output.Fail/Failf,
// or a real SIGINT/SIGTERM cancellation via Output.Cancel/TaskHandle.Cancel),
// not merely a caller who forgot to resolve a task. It gates whether a
// leftover Running task may still read as Cancelled/130 (release-gate
// finding 1).
func (o *Output) abnormalFinishLocked() bool {
	for _, t := range o.tasks {
		if t.state == Failed || t.state == Cancelled {
			return true
		}
	}
	return false
}

// unresolvedTaskHint names the concrete corrective action for a task Finish
// found with no final state — rendered as a "→" conclusion action the same
// way Confirm's own policy hint renders (TaskHandle.Next), replacing the raw
// "misuse: <name>: evo: ..." sentinel text that told the reader nothing
// about what to do next (release-gate finding 3).
const unresolvedTaskHint = "call Done, Fail, Block, Skipped, or a mutation verb on this task"

// attachUnresolvedTaskHintLocked attaches unresolvedTaskHint to t directly.
// It cannot go through TaskHandle.Next, which refuses once Finish has set
// o.finishing — this runs from inside Finish's own unresolved-task sweep.
func attachUnresolvedTaskHintLocked(t *taskState) {
	t.actions = append(t.actions, Label(unresolvedTaskHint))
}

// autoResolveGroupsLocked stops each group from implying "still might run"
// once a member has already failed or been cancelled: every declared-after
// sibling that has not reached its own terminal state becomes NotStarted.
// A sibling the caller already resolved (explicitly or by an earlier trigger)
// is left untouched — explicit resolution always wins.
func (o *Output) autoResolveGroupsLocked() {
	for _, col := range o.collections {
		if !col.sequential {
			continue
		}
		triggered := false
		for _, t := range col.tasks {
			if !triggered {
				if t.state == Failed || t.state == Cancelled {
					triggered = true
				}
				continue
			}
			if core.IsTerminalTask(t.state) {
				continue
			}
			t.state = NotStarted
			t.phase = ""
			t.summary = notStartedSummary
			o.appendEventLocked(Event{Type: "task.not_started", EntityID: t.id})
		}
	}
}

// Finish validates, computes conclusion, emits final projections.
// Projection I/O runs outside the domain lock (§17.1).
func (o *Output) Finish() error {
	o.drainScheduler()
	o.mu.Lock()
	if o.finished {
		err := o.misuse
		o.mu.Unlock()
		return err
	}
	o.finishing = true

	// Flush unterminated Print fragments into messages.
	o.flushPendingPrintLocked()

	// Group lifecycle: a failed/cancelled child stops its later siblings from
	// reading as "still pending" before the generic unresolved-entity sweep
	// below would otherwise mark them Incomplete (and record misuse).
	o.autoResolveGroupsLocked()

	// Unresolved entities. A task with no problems of its own told an
	// honest, complete story already — the caller just never called a
	// terminal verb — whenever it also carries at least one of: a recorded
	// mutation-verb effect (Delete/Create/Update/...), a sealed absolute
	// progress (a completed Each/EachN/Progress loop reached its total),
	// recorded taxonomy (Skipped/Kept), or a recorded warning (P2:
	// TaskHandle.Warn never itself resolves the task, so a warned-but-
	// unresolved task earns the same amnesty). The easiest path (forgetting
	// Done) becomes correct instead of a surprising Cancelled/NotStarted
	// plus a silent exit-code flip (beginner-1, I1; beginner-gate-2 findings
	// 1/2/4). Anything else still reads as misuse, but now names the task
	// so Finish can render it.
	abnormal := o.abnormalFinishLocked()
	for _, t := range o.tasks {
		if core.IsTerminalTask(t.state) {
			continue
		}
		if len(t.problems) == 0 && (o.hasRecordedEffectLocked(t.name) || hasSealedProgress(t) || hasRecordedTaxonomy(t) || len(t.warnings) > 0) {
			t.state = Done
			t.phase = ""
			o.appendEventLocked(Event{Type: "task.done", EntityID: t.id})
			continue
		}
		if !abnormal {
			// Clean, unsignalled, error-free finish: a forgotten terminal
			// verb told an incomplete story, not a caller bug — never
			// Cancelled/130 (release-gate finding 1), and never misuse-driven
			// exit escalation regardless of whether the task ever reached
			// Running (release-gate round 4 finding 3: a Phase call that
			// promoted it to Running before it was abandoned must not flip
			// the exit code against an identical task that was never
			// touched at all — same amnesty qualifier, same outcome). No
			// misuse recorded: this is an honest partial outcome (folded
			// into Conclusion.Partial), not bookkeeping the caller must fix.
			// The hint still names the corrective action either way.
			t.state = Incomplete
			t.phase = ""
			t.summary = unresolvedTaskIncompleteSummary
			attachUnresolvedTaskHintLocked(t)
			continue
		}
		resolveUnstartedTaskLocked(t)
		// A declared-but-never-defined task never started — Each children
		// the loop did not yield, or work the interrupt took away. That is
		// the answer, not misuse.
		if t.state == NotStarted {
			continue
		}
		o.recordMisuseFor(t.name, ErrUnresolvedTask)
		attachUnresolvedTaskHintLocked(t)
	}
	if o.misuse != nil && !errors.Is(o.misuse, ErrUnresolvedTask) {
		o.appendMisuseLineLocked()
	}

	snap := o.snapshotLocked()
	conc := core.InferConclusion(snap)
	core.FoldLeftoverMisuse(&conc, o.misuse)
	core.ApplyFailedExitCode(&conc, o.cfg.failedExitCode)
	conc.RunID = o.outputID
	conc.StartedAt = o.startedAt
	conc.FinishedAt = o.cfg.clock.Now()
	o.conclusion = &conc
	snap.Conclusion = &conc
	o.appendEventLocked(Event{
		Type:  "output.finished",
		State: string(conc.State),
	})
	writer := o.cfg.primary
	cfg := o.cfg
	misuse := o.misuse
	o.finished = true
	o.finishing = false

	// FormatJSON's one final "evo.run" document (spec §32.1) — independent
	// of the legacy projection.suppressesHuman() branch below, since human
	// presentation still streams to Stderr for this Format (§32.1: "stderr:
	// human live/plain presentation ... never mixed into stdout"). A write
	// failure here is a real Run failure (§32.2), folded into misuse so
	// every return path below already carries it.
	if cfg.wireFormat == FormatJSON {
		if err := writeWireRunLocked(cfg.wireStream, conc); err != nil {
			if misuse == nil {
				misuse = err
			} else {
				misuse = errors.Join(misuse, err)
			}
		}
	}

	if cfg.projection.suppressesHuman() {
		var events []Event
		if cfg.projection == ProjectionJSONL {
			events = append([]Event(nil), o.events...)
		}
		o.mu.Unlock()
		return writeMachinePresentation(writer, snap, events, cfg.projection, misuse)
	}

	// Captured before residualPlainLocked drains o.linesEmitted for its own
	// copy, so residualInteractiveFinalLocked's copy (below) sees the same
	// unemitted tail instead of finding it already consumed (release-gate
	// round 5 finding 1).
	linesFrom := o.linesEmitted
	// Human stream: only residual (terminal outcomes already streamed).
	residual := o.residualPlainLocked(snap)
	interactive := false
	if live := o.liveLocked(); live != nil && live.IsInteractive() && !cfg.plain {
		interactive = true
		// Interactive final: conclusion + any unemitted entities (not a second full dump).
		o.finishLiveLocked(o.residualInteractiveFinalLocked(snap, linesFrom))
	}
	o.mu.Unlock()

	// CON-009: fan-out residual to primary + AlsoWrite when not already on the live driver.
	// Interactive path already wrote durable items + WriteFinal; skip full dump to primary
	// unless a primary writer is configured for a second stream (stdout purity dual-write).
	var writeErr error
	if !interactive {
		writers := make([]io.Writer, 0, 1+len(cfg.extraWriters))
		if writer != nil {
			writers = append(writers, writer)
		}
		writers = append(writers, cfg.extraWriters...)
		for _, w := range writers {
			if _, err := io.WriteString(w, residual); err != nil && writeErr == nil {
				writeErr = fmt.Errorf("%w: %v", ErrRenderer, err)
			}
			if f, ok := w.(flusher); ok {
				_ = f.Flush()
			}
		}
	} else {
		// Dual stream: residual conclusion on primary (items already durable on
		// terminal). Primary is skipped when it and the live terminal share one
		// physical writer (default construction) — the terminal's WriteFinal
		// already rendered this conclusion band, so writing it again to primary
		// would duplicate it on the same screen. AlsoWrite mirrors are never
		// skipped: they are a distinct stream from the terminal by definition,
		// and option.go's AlsoWrite promises the plain projection regardless of
		// interactive/plain (X4).
		writers := make([]io.Writer, 0, 1+len(cfg.extraWriters))
		if writer != nil && !cfg.samePrimaryAsTerminal {
			writers = append(writers, writer)
		}
		writers = append(writers, cfg.extraWriters...)
		for _, w := range writers {
			if _, err := io.WriteString(w, residual); err != nil && writeErr == nil {
				writeErr = fmt.Errorf("%w: %v", ErrRenderer, err)
			}
			if f, ok := w.(flusher); ok {
				_ = f.Flush()
			}
		}
	}
	if writeErr != nil {
		if misuse == nil {
			return writeErr
		}
		return errors.Join(misuse, writeErr)
	}
	return misuse
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
	if manifestStore != nil {
		// Releases this Run's exclusive manifest lock (spec §11.3). Already
		// committed Task records on disk are unaffected — Close never rolls
		// anything back.
		_ = manifestStore.Close()
	}
	return nil
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
	core.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
	return c
}

// Events returns a copy of durable events (v0.1 journal).
func (o *Output) copyEvents() []Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Event, len(o.events))
	copy(out, o.events)
	return out
}
