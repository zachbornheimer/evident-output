package evo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Output is the aggregate root for one command's presentation lifecycle.
type Output struct {
	mu sync.Mutex

	cfg config

	outputID  string
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

	// namedTasks backs get-or-create identity for Output.Task/Scope.Task
	// (and, through them, the package-level default-instance facade): a
	// repeated (scope, name) pair — or a repeated explicit evo.ID reused
	// under the same name — returns the same handle instead of a duplicate
	// row. Keyed either "\x00"+scope+"\x00"+name (no explicit ID) or
	// "key:"+key (explicit evo.ID); see taskScoped and taskNameByKey.
	namedTasks map[string]*TaskHandle
	// taskNameByKey remembers which display name first claimed an explicit
	// evo.ID through taskScoped, so a second call under the SAME name gets
	// the live handle (L1's get-or-create fix) while a second call reusing
	// the ID under a DIFFERENT name still reports ErrDuplicateKey — a real
	// identity conflict, not a repeat declaration.
	taskNameByKey map[string]string
	// namedPlans/namedChanges back get-or-create identity for TaskHandle
	// mutation verbs (Delete, Create, ...): repeated mutations on one task
	// accumulate into the one Plan/Changes section named after the task,
	// instead of one section per call.
	namedPlans   map[string]*Plan
	namedChanges map[string]*Changes
	// namedReasons backs get-or-create identity for evo.Reason: repeated calls
	// with the same name (inline or lifted to a var) merge into one bucket.
	namedReasons map[string]TaxonomyReason
	// namedGroups backs get-or-create identity for evo.Group: repeated calls
	// with the same name return the one Group instead of a duplicate section.
	namedGroups map[string]*GroupHandle

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

	// activityAt is the domain-clock time of the most recent Phase or Progress
	// call. The live renderer diffs against it to grow a heartbeat suffix
	// ("pushing feat/a — 90s") once a Running task's phase has gone stale, and
	// it resets on every Phase/Progress update (see phaseStaleAfter in live.go).
	activityAt time.Time

	// capture is the get-or-create sink shared by Task.Capture and PhaseWriter
	// so child-process evidence recorded via either path lands in one ring and
	// DetailTail sees it after Fail.
	evidence *Evidence

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

	// unchanged marks a Done task resolved via Task.Unchanged/Unchangedf
	// (I7) — "checked, nothing needed to change" — distinct from an
	// ordinary Done's generic "ready" verdict. inferConclusion derives
	// StateUnchanged over the generic StateReady when every Done-family
	// task in the run carries this tag and nothing was recorded changed.
	unchanged bool

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
}

type tasksState struct {
	id          string
	name        string
	summary     string
	tasks       []*taskState
	declaration int
	handle      *Tasks

	// namedTasks backs get-or-create identity for Group.Task: repeated calls
	// with the same name inside this group return the one child TaskHandle.
	namedTasks map[string]*TaskHandle

	// sequential marks a collection whose children are declared as a
	// sequence of steps (Group), where the "one Running child" heart
	// contract (evo-rec.md) applies. A plain Tasks collection documents
	// its children as independent — worker-pool fan-out is a supported,
	// concurrency-safe pattern there, so promoteRunningLocked does not
	// police it.
	sequential bool
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
	handle       *Changes
}

type planState struct {
	id      string
	subject string
	records []EffectRecord
	// intendedVerb mirrors changesState.intendedVerb for plan sections.
	intendedVerb string
	handle       *Plan
}

func newOutput(subject string, options ...Option) *Output {
	cfg := config{
		subject:         subject,
		clock:           SystemClock{},
		visibilityDelay: defaultVisibilityDelay,
		maxFrameRate:    defaultMaxFrameRate,
		width:           defaultWidth,
		debugLevel:      LevelInfo,
		redactor:        NoopRedactor{},
		maxEntities:     defaultMaxEntities,
		verbosity:       VerbosityNormal,
	}
	for _, opt := range options {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	// A Terminal driver supplied without To() must still land its
	// non-interactive/residual projection somewhere (release-gate round 8
	// finding 2): default primary to the driver's own Sink() when it
	// reports one. A driver that explicitly reports no fixed sink (nil —
	// e.g. a virtual test screen driving output straight through the live
	// surface) is left alone; a driver that cannot answer the question at
	// all is misuse, recorded below once o exists.
	//
	// Whenever the driver's sink IS the primary writer — whether that's this
	// same defaulting, or an explicit To()/Diagnostics() (Options path) or
	// Config's own To(c.Stdout)/To(c.Stderr) wiring (Config.Terminal path,
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
	// An Options build that installs neither To() nor a Terminal sink must
	// still land its residual/conclusion projection somewhere — before this,
	// primary stayed nil and Finish silently wrote zero bytes, even on a
	// Fail (exit 2 with no evidence of why). Default to os.Stdout, matching
	// the non-Options default, and apply the same TTY/color inference the
	// non-Options path applies to that stream: never override an explicit
	// NoColor(), only ever strengthen it, so a defaulted destination piped
	// to a file never leaks raw ANSI into it (release-gate round 9 findings
	// 2 and 5).
	if cfg.terminal == nil && cfg.primary == nil {
		cfg.primary = os.Stdout
		if !cfg.noColor && (os.Getenv("NO_COLOR") != "" || !IsCharDevice(cfg.primary)) {
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
	o := &Output{
		cfg:        cfg,
		outputID:   "out_1",
		taskByRef:  make(map[string]*taskState),
		tasksByRef: make(map[string]*tasksState),
		keys:       make(map[string]struct{}),
	}
	// Stable-enough id for a process-local output instance.
	o.outputID = o.nextID("out")
	o.appendEventLocked(Event{Type: "output.started", OutputID: o.outputID})
	if terminalWithoutSink {
		o.recordMisuse(ErrTerminalWithoutSink)
	}
	if cfg.dryRun {
		// Announce before any task/item can reach the durable stream: no
		// caller path can finish a DryRun-configured Output without this
		// line having appeared first (evo-rec.md Problem 1). Safe to write
		// unlocked — o has not yet been returned to the caller.
		o.emitDryRunMarkerLocked()
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
func (o *Output) DeclareDryRun() {
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
	o.emitDryRunMarkerLocked()
}

// emitDryRunMarkerLocked writes the "[dry-run]  no changes will be made"
// announcement immediately, once, through the same durable-write path
// (writeDurableTextLocked) every other library-owned line uses.
func (o *Output) emitDryRunMarkerLocked() {
	var b strings.Builder
	writeDryRunMarker(&b, !o.cfg.noColor)
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
	glyph := styleGlyph(misuseGlyph, sgrYellow, !o.cfg.noColor)
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
// write). For a sequential collection (Group), it records misuse when a
// sibling is already Running, enforcing the heart contract "one Running
// child" (evo-rec.md) — callers still get the transition; Strict mode is
// what escalates the violation to a panic. A plain Tasks collection
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
func (o *Output) Task(name string, args ...any) *TaskHandle {
	formatted, opts := formatEntityName(name, args)
	return o.taskScoped(formatted, "", opts...)
}

// taskScoped is the get-or-create identity behind Output.Task and Scope.Task:
// a repeated call with the same (scope, name) pair returns the live handle
// instead of declaring a duplicate row — the same contract evo.Task already
// gives the package-level default instance. An explicit evo.ID reused under
// a different name is still a real identity conflict and reports
// ErrDuplicateKey (addTaskLocked's own key bookkeeping catches it, since a
// differing identity never short-circuits through the namedTasks cache
// below).
func (o *Output) taskScoped(name, scope string, opts ...EntityOption) *TaskHandle {
	eo := applyEntityOptions(opts)
	clean := sanitize.Text(name)
	key := qualifyKey(scope, eo.key)

	o.mu.Lock()
	defer o.mu.Unlock()

	if key != "" {
		if ownerName, ok := o.taskNameByKey[key]; ok {
			if ownerName == clean {
				if existing, ok := o.namedTasks["key:"+key]; ok {
					return existing
				}
			}
			o.recordMisuse(ErrDuplicateKey)
			return &TaskHandle{out: o, id: o.nextID("task")}
		}
	} else if existing, ok := o.namedTasks["\x00"+scope+"\x00"+clean]; ok {
		return existing
	}

	h := o.addTaskLocked(clean, nil, key)
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
		if st := o.taskByRef[h.id]; st != nil && o.ensureOpen() == nil && !isTerminalTask(st.state) {
			o.setPhaseLocked(st, eo.phase)
		}
	}
	return h
}

func (o *Output) addTaskLocked(name string, col *tasksState, key string) *TaskHandle {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	if key != "" {
		if _, ok := o.keys[key]; ok {
			o.recordMisuse(ErrDuplicateKey)
			return &TaskHandle{out: o, id: o.nextID("task")}
		}
		o.keys[key] = struct{}{}
	}
	if err := o.ensureEntityRoomLocked(); err != nil {
		o.recordMisuse(err)
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	st := &taskState{
		id:          o.nextID("task"),
		key:         key,
		name:        name,
		state:       Pending,
		progress:    Progress{Kind: Indeterminate},
		collection:  col,
		declaration: o.nextDecl(),
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
	o.signalLiveLocked(true)
	return h
}

// taskGetOrCreate returns the task previously created under name by this
// method, or declares a new one — the identity backing the package-level
// evo.Task(name) facade, where repeated calls must return the same handle
// instead of the duplicate-key error Task/ID would raise.
func (o *Output) taskGetOrCreate(name string, opts ...EntityOption) *TaskHandle {
	return o.taskScoped(name, "", opts...)
}

// planGetOrCreate returns the Plan previously created under subject by this
// method, or declares a new one — the identity backing TaskHandle mutation
// verbs, where repeated dry-run mutations on one task accumulate into one
// [planned] section instead of a new one per call.
func (o *Output) planGetOrCreate(subject string) *Plan {
	o.mu.Lock()
	if p, ok := o.namedPlans[subject]; ok {
		o.mu.Unlock()
		return p
	}
	o.mu.Unlock()

	p := o.Plan(subject)

	o.mu.Lock()
	if existing, ok := o.namedPlans[subject]; ok {
		o.mu.Unlock()
		return existing
	}
	if o.namedPlans == nil {
		o.namedPlans = make(map[string]*Plan)
	}
	o.namedPlans[subject] = p
	o.mu.Unlock()
	return p
}

// changesGetOrCreate is planGetOrCreate's counterpart for applied mutations.
func (o *Output) changesGetOrCreate(subject string) *Changes {
	o.mu.Lock()
	if c, ok := o.namedChanges[subject]; ok {
		o.mu.Unlock()
		return c
	}
	o.mu.Unlock()

	c := o.Changes(subject)

	o.mu.Lock()
	if existing, ok := o.namedChanges[subject]; ok {
		o.mu.Unlock()
		return existing
	}
	if o.namedChanges == nil {
		o.namedChanges = make(map[string]*Changes)
	}
	o.namedChanges[subject] = c
	o.mu.Unlock()
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
func (o *Output) cancelActive(reason string) {
	o.mu.Lock()
	if o.cancelPendingConfirmLocked(reason) {
		o.mu.Unlock()
		return
	}
	var active *TaskHandle
	for _, t := range o.tasks {
		if t.state == Running {
			active = t.handle
			break
		}
	}
	if active == nil {
		// Nothing has reached Running yet: the earliest-declared Pending
		// task is the one about to run next (evo-rec.md "one Running
		// child" — pending siblings are named and idle, waiting their
		// turn), so an interrupt before any evidence still cancels that
		// task rather than falling through to Output-level cancel.
		for _, t := range o.tasks {
			if t.state == Pending {
				active = t.handle
				break
			}
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
		if st := o.taskByRef[id]; st != nil && !isTerminalTask(st.state) {
			st.state = Cancelled
			st.summary = sanitize.Text(reason)
			o.bumpLocked()
			o.appendEventLocked(Event{Type: "task.cancelled", EntityID: id})
			o.commitResolvedTaskLocked(id)
		}
		return true
	}
	return false
}

// Tasks declares a collection of independent child tasks. name is a printf
// format when args are present (fmt.Sprintf semantics) — one text spelling
// shared with Task/Group/Reason (C6); no args leaves name untouched.
func (o *Output) Tasks(name string, args ...any) *Tasks {
	if len(args) > 0 {
		name = fmt.Sprintf(name, args...)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &Tasks{out: o, id: o.nextID("tasks")}
	}
	st := &tasksState{
		id:          o.nextID("tasks"),
		name:        sanitize.Text(name),
		declaration: o.nextDecl(),
	}
	h := &Tasks{out: o, id: st.id}
	st.handle = h
	o.collections = append(o.collections, st)
	o.tasksByRef[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	return h
}

// Group declares (or, for a repeated name, returns) a self-managing task
// group — the front door for a sequence of steps that must stop implying
// "still might run" once a member has already failed or been cancelled. A
// second evo.Group("python") call returns the same Group, mirroring Task's
// get-or-create identity. name is a printf format when args are present
// (fmt.Sprintf semantics); no args leaves name untouched.
func (o *Output) Group(name string, args ...any) *GroupHandle {
	if len(args) > 0 {
		name = fmt.Sprintf(name, args...)
	}
	o.mu.Lock()
	if g, ok := o.namedGroups[name]; ok {
		o.mu.Unlock()
		return g
	}
	o.mu.Unlock()

	tasks := o.Tasks(name)

	o.mu.Lock()
	defer o.mu.Unlock()
	if existing, ok := o.namedGroups[name]; ok {
		return existing
	}
	if col := o.tasksByRef[tasks.id]; col != nil {
		col.sequential = true
	}
	g := &GroupHandle{tasks: tasks}
	if o.namedGroups == nil {
		o.namedGroups = make(map[string]*GroupHandle)
	}
	o.namedGroups[name] = g
	return g
}

// groupTaskGetOrCreate returns the child previously declared under name in
// the group backed by groupID, or declares a new one — the identity behind
// Group.Task's get-or-create contract.
func (o *Output) groupTaskGetOrCreate(groupID, name string, opts ...EntityOption) *TaskHandle {
	o.mu.Lock()
	col := o.tasksByRef[groupID]
	if col == nil {
		o.mu.Unlock()
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	if existing, ok := col.namedTasks[name]; ok {
		o.mu.Unlock()
		return existing
	}
	eo := applyEntityOptions(opts)
	h := o.addTaskLocked(sanitize.Text(name), col, eo.key)
	if col.namedTasks == nil {
		col.namedTasks = make(map[string]*TaskHandle)
	}
	col.namedTasks[name] = h
	o.mu.Unlock()
	return h
}

// Changes starts a durable-effects section. subject is a printf format
// when args are present (fmt.Sprintf semantics) — see Tasks (C6).
func (o *Output) Changes(subject string, args ...any) *Changes {
	if len(args) > 0 {
		subject = fmt.Sprintf(subject, args...)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &Changes{out: o, id: o.nextID("changes")}
	}
	st := &changesState{
		id:      o.nextID("changes"),
		subject: sanitize.Text(subject),
	}
	h := &Changes{out: o, id: st.id}
	st.handle = h
	o.changes = append(o.changes, st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "changes.declared", EntityID: st.id})
	return h
}

// Plan starts a would-occur effects section. subject is a printf format
// when args are present (fmt.Sprintf semantics) — see Tasks (C6).
func (o *Output) Plan(subject string, args ...any) *Plan {
	if len(args) > 0 {
		subject = fmt.Sprintf(subject, args...)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &Plan{out: o, id: o.nextID("plan")}
	}
	st := &planState{
		id:      o.nextID("plan"),
		subject: sanitize.Text(subject),
	}
	h := &Plan{out: o, id: st.id}
	st.handle = h
	o.plans = append(o.plans, st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "plan.declared", EntityID: st.id})
	return h
}

// Fail records an output-level failure.
func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.failWith(applyProblemOptions(sanitize.Text(summary), options))
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
	summary, evidence := splitWrappedMessage(format, err)
	o.failWith(sanitizeProblem(Problem{Summary: summary, Detail: evidence}))
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
		name:        sanitize.Text(o.cfg.subject),
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
	name := sanitize.Text(o.cfg.subject)
	if name == "" {
		name = identityFallbackName()
	}
	t := &taskState{
		id:          o.nextID("task"),
		name:        name,
		state:       Cancelled,
		summary:     sanitize.Text(reason),
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
func (o *Output) Debug(message string, fields ...Field) {
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
		Message: sanitize.Text(message),
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
		// (the realistic default: Config{Stdout, Stderr} routes To(Stdout),
		// Diagnostics(Stderr)).
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
		Version:   o.version,
		OutputID:  o.outputID,
		Subject:   o.cfg.subject,
		Lines:     append([]string(nil), o.lines...),
		Actions:   cloneActions(o.collectActionsLocked()),
		Timestamp: o.cfg.clock.Now(),
		DryRun:    o.cfg.dryRun,
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
	return TaskSnapshot{
		ID:          t.id,
		Key:         t.key,
		Name:        t.name,
		State:       t.state,
		Phase:       t.phase,
		ActivityAt:  t.activityAt,
		Progress:    t.progress,
		Summary:     t.summary,
		Problems:    cloneProblems(t.problems),
		Actions:     cloneActions(t.actions),
		Skipped:     cloneTaxonomy(t.skipped),
		Kept:        cloneTaxonomy(t.kept),
		Collection:  colID,
		Declaration: t.declaration,
		synthetic:   t.synthetic,
		unchanged:   t.unchanged,
	}
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
		Name:        g.name,
		State:       g.derivedState(),
		Summary:     g.displaySummary(),
		Declaration: g.declaration,
	}
	for _, t := range g.tasks {
		ts.Tasks = append(ts.Tasks, t.snapshot())
	}
	return ts
}

func (g *tasksState) derivedState() EntityState {
	if len(g.tasks) == 0 {
		return Empty
	}
	var anyRunning, anyFailed, anyWarning, anyCancelled, anyUnresolved bool
	allDone := true
	for _, t := range g.tasks {
		switch t.state {
		case Running:
			anyRunning = true
			allDone = false
		case Pending:
			anyUnresolved = true
			allDone = false
		case Failed:
			anyFailed = true
		case Warning:
			anyWarning = true
		case Cancelled:
			anyCancelled = true
		case Done, Skipped:
		case NotStarted:
			// Auto-resolved because an earlier sibling already failed/cancelled —
			// not a source of incompleteness; the group's verdict already comes
			// from that sibling.
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
	if anyWarning {
		return Warning
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
	if st == Done && g.summary != "" {
		// only if no failures/warnings
		for _, t := range g.tasks {
			if t.state == Failed || t.state == Warning || t.state == Cancelled {
				return ""
			}
		}
		return g.summary
	}
	return ""
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
const unresolvedTaskHint = "call Done, Fail, Block, Skip, or a mutation verb on this task"

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
		triggered := false
		for _, t := range col.tasks {
			if !triggered {
				if t.state == Failed || t.state == Cancelled {
					triggered = true
				}
				continue
			}
			if isTerminalTask(t.state) {
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
	// progress (a completed Each/EachN/Progress loop reached its total), or
	// recorded taxonomy (Skipped/Kept). The easiest path (forgetting Done)
	// becomes correct instead of a surprising Cancelled/NotStarted plus a
	// silent exit-code flip (beginner-1, I1; beginner-gate-2 findings 1/2/4).
	// Anything else still reads as misuse, but now names the task so Finish
	// can render it.
	abnormal := o.abnormalFinishLocked()
	for _, t := range o.tasks {
		if isTerminalTask(t.state) {
			continue
		}
		if len(t.problems) == 0 && (o.hasRecordedEffectLocked(t.name) || hasSealedProgress(t) || hasRecordedTaxonomy(t)) {
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
		o.recordMisuseFor(t.name, ErrUnresolvedTask)
		attachUnresolvedTaskHintLocked(t)
	}
	if o.misuse != nil && !errors.Is(o.misuse, ErrUnresolvedTask) {
		o.appendMisuseLineLocked()
	}

	snap := o.snapshotLocked()
	conc := inferConclusion(snap)
	foldLeftoverMisuseLocked(&conc, o.misuse)
	applyFailedExitCode(&conc, o.cfg.failedExitCode)
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
	o.mu.Unlock()
	return nil
}

// Conclusion returns the computed conclusion after Finish.
func (o *Output) Conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.conclusion != nil {
		return *o.conclusion
	}
	snap := o.snapshotLocked()
	c := inferConclusion(snap)
	applyFailedExitCode(&c, o.cfg.failedExitCode)
	return c
}

// Events returns a copy of durable events (v0.1 journal).
func (o *Output) Events() []Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Event, len(o.events))
	copy(out, o.events)
	return out
}
