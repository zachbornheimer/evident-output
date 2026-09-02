package evo

import (
	"errors"
	"fmt"
	"io"
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

	outputID   string
	idSeq      uint64
	declSeq    int
	version    uint64
	closed     bool
	finishing  bool
	finished   bool
	armed      bool // set by arm(): live surface may paint before any entity exists
	misuse     error
	conclusion *Conclusion
	finalPlain string
	live       *liveEngine
	snapCh     chan Snapshot

	items       []*itemState
	tasks       []*taskState
	collections []*tasksState
	changes     []*changesState
	plans       []*planState
	lines       []string
	actions     []Action
	events      []Event

	itemByRef  map[string]*itemState
	taskByRef  map[string]*taskState
	tasksByRef map[string]*tasksState
	keys       map[string]struct{}

	// namedTasks backs get-or-create identity for the package-level default
	// instance facade: evo.Task(name) called twice returns the same handle.
	namedTasks map[string]*TaskHandle
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

	// Structured debug journal (§21.3). lines[] still holds history-format strings
	// for FinalPlain / residual compatibility.
	debugRecords []debugRecord
	// debugPaneActive is true once Debug was projected via the live rolling pane
	// (not history fallback). Controls failure diagnostic-tail eligibility.
	debugPaneActive bool

	// Print/Printf/Println line buffers and canonical messages.
	pendingPrint strings.Builder
	pendingVis   Visibility // visibility of the current pending fragment
	messages     []messageState
}

type itemState struct {
	id          string
	key         string // optional stable machine key (platform ID)
	name        string
	state       EntityState
	problems    []Problem
	because     string
	actions     []Action
	declaration int
	handle      *ItemHandle

	// Emission bookkeeping so resolved items are not buffered until Finish.
	coreEmitted    bool
	becauseEmitted bool
	actionsEmitted int
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
	capture *Capture

	// skipped/kept hold disposition taxonomy accumulated by Skipped/Kept —
	// the model that "! skipped N (...)" / "! kept N (...)" are derived from
	// at render time, never a hand-built summary string. Disposition side of
	// the model, not the mutation ledger (Plan/Changes).
	skipped []TaxonomyRecord
	kept    []TaxonomyRecord

	// Emission bookkeeping so terminal standalone tasks stream in plain mode
	// on resolve (P2) — same spirit as itemState.coreEmitted.
	coreEmitted bool
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
	handle  *Changes
}

type planState struct {
	id      string
	subject string
	records []EffectRecord
	handle  *Plan
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
		itemByRef:  make(map[string]*itemState),
		taskByRef:  make(map[string]*taskState),
		tasksByRef: make(map[string]*tasksState),
		keys:       make(map[string]struct{}),
	}
	// Stable-enough id for a process-local output instance.
	o.outputID = o.nextID("out")
	o.appendEventLocked(Event{Type: "output.started", OutputID: o.outputID})
	if cfg.dryRun {
		// Announce before any task/item can reach the durable stream: no
		// caller path can finish a DryRun-configured Output without this
		// line having appeared first (evo-rec.md Problem 1). Safe to write
		// unlocked — o has not yet been returned to the caller.
		o.emitDryRunMarkerLocked()
	}
	return o
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

// Item declares a named final-report condition. Optional evo.ID sets a stable machine key.
func (o *Output) Item(name string, opts ...EntityOption) *ItemHandle {
	return o.itemScoped(name, "", opts...)
}

func (o *Output) itemScoped(name, scope string, opts ...EntityOption) *ItemHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	eo := applyEntityOptions(opts)
	key := qualifyKey(scope, eo.key)
	return o.addItemLocked(sanitize.Text(name), key, 0)
}

func (o *Output) addItemLocked(name, key string, order int) *ItemHandle {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &ItemHandle{out: o, id: o.nextID("item")}
	}
	if key != "" {
		if _, ok := o.keys[key]; ok {
			o.recordMisuse(ErrDuplicateKey)
			return &ItemHandle{out: o, id: o.nextID("item")}
		}
		o.keys[key] = struct{}{}
	}
	if err := o.ensureEntityRoomLocked(); err != nil {
		o.recordMisuse(err)
		return &ItemHandle{out: o, id: o.nextID("item")}
	}
	st := &itemState{
		id:          o.nextID("item"),
		key:         key,
		name:        name,
		state:       Pending,
		declaration: o.nextDecl(),
	}
	if order != 0 {
		st.declaration = order
	}
	h := &ItemHandle{out: o, id: st.id}
	st.handle = h
	o.items = append(o.items, st)
	o.itemByRef[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "item.declared", EntityID: st.id})
	return h
}

func (o *Output) ensureEntityRoomLocked() error {
	n := len(o.items) + len(o.tasks)
	if n >= o.cfg.maxEntities {
		return ErrLimitExceeded
	}
	return nil
}

// Task declares a single operation. Optional evo.ID sets a stable machine key.
func (o *Output) Task(name string, opts ...EntityOption) *TaskHandle {
	return o.taskScoped(name, "", opts...)
}

func (o *Output) taskScoped(name, scope string, opts ...EntityOption) *TaskHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	eo := applyEntityOptions(opts)
	key := qualifyKey(scope, eo.key)
	return o.addTaskLocked(sanitize.Text(name), nil, key)
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
	o.mu.Lock()
	if t, ok := o.namedTasks[name]; ok {
		o.mu.Unlock()
		return t
	}
	o.mu.Unlock()

	t := o.Task(name, opts...)

	o.mu.Lock()
	if existing, ok := o.namedTasks[name]; ok {
		o.mu.Unlock()
		return existing
	}
	if o.namedTasks == nil {
		o.namedTasks = make(map[string]*TaskHandle)
	}
	o.namedTasks[name] = t
	o.mu.Unlock()
	return t
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
func (o *Output) cancelActive(reason string) {
	o.mu.Lock()
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
	if o.cancelPendingConfirmLocked(reason) {
		o.mu.Unlock()
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
		if st := o.itemByRef[id]; st != nil && !isTerminalItem(st.state) {
			st.state = Cancelled
			st.because = sanitize.Text(reason)
			o.bumpLocked()
			o.appendEventLocked(Event{Type: "item.cancelled", EntityID: id})
			o.emitItemProgressiveLocked(st)
		}
		return true
	}
	return false
}

// Tasks declares a collection of independent child tasks.
func (o *Output) Tasks(name string) *Tasks {
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
// get-or-create identity.
func (o *Output) Group(name string) *GroupHandle {
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

// Changes starts a durable-effects section.
func (o *Output) Changes(subject string) *Changes {
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

// Plan starts a would-occur effects section.
func (o *Output) Plan(subject string) *Plan {
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	// Synthetic failed item for conclusion.
	st := &itemState{
		id:          o.nextID("item"),
		name:        sanitize.Text(o.cfg.subject),
		state:       Failed,
		problems:    []Problem{applyProblemOptions(sanitize.Text(summary), options)},
		declaration: o.nextDecl(),
	}
	if st.name == "" {
		st.name = "command"
	}
	o.items = append(o.items, st)
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
	t := &taskState{
		id:          o.nextID("task"),
		name:        "command",
		state:       Cancelled,
		summary:     sanitize.Text(reason),
		declaration: o.nextDecl(),
	}
	o.tasks = append(o.tasks, t)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.cancelled"})
}

// Explain sets an explicit conclusion explanation (applied at Finish).
func (o *Output) Explain(text string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if o.conclusion == nil {
		o.conclusion = &Conclusion{}
	}
	o.conclusion.Explanation = sanitize.Text(text)
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
	if o.cfg.diagnostic != nil {
		o.writeDiagnosticTextLocked(plainHistory + "\n")
	}
	if dual {
		o.linesEmitted = len(o.lines)
		return
	}

	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain && !o.cfg.nonInteractive
	if interactive && o.cfg.debugPresentation == DebugPresentationPane {
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
	for _, it := range o.items {
		s.Items = append(s.Items, it.snapshot())
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

func (it *itemState) snapshot() ItemSnapshot {
	return ItemSnapshot{
		ID:          it.id,
		Key:         it.key,
		Name:        it.name,
		State:       it.state,
		Problems:    cloneProblems(it.problems),
		Because:     it.because,
		Actions:     cloneActions(it.actions),
		Declaration: it.declaration,
	}
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
	}
}

func cloneTaxonomy(in []TaxonomyRecord) []TaxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]TaxonomyRecord, len(in))
	copy(out, in)
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
		ID:      c.id,
		Subject: c.subject,
		Records: append([]EffectRecord(nil), c.records...),
	}
}

func (p *planState) snapshot() PlanSnapshot {
	return PlanSnapshot{
		ID:      p.id,
		Subject: p.subject,
		Records: append([]EffectRecord(nil), p.records...),
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
	for _, it := range o.items {
		add(it.actions)
		for _, p := range it.problems {
			add(p.Actions)
		}
	}
	for _, t := range o.tasks {
		add(t.actions)
	}
	return out
}

func (o *Output) bumpLocked() {
	o.version++
	o.publishSnapshotLocked()
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
		"item.blocked", "item.failed", "task.failed", "output.started":
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

	// Unresolved entities
	for _, it := range o.items {
		if !isTerminalItem(it.state) {
			it.state = Incomplete
			o.recordMisuse(ErrUnresolvedItem)
		}
	}
	for _, t := range o.tasks {
		if !isTerminalTask(t.state) {
			t.state = Incomplete
			o.recordMisuse(ErrUnresolvedTask)
		}
	}

	snap := o.snapshotLocked()
	conc := inferConclusion(snap)
	applyFailedExitCode(&conc, o.cfg.failedExitCode)
	if o.conclusion != nil && o.conclusion.Explanation != "" {
		conc.Explanation = o.conclusion.Explanation
	}
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

	// Full plain for FinalPlain / JSON agreement (may include already-streamed items).
	fullPlain, _ := RenderPlain(snap, PlainOptions{
		Width: cfg.width, NoColor: cfg.noColor, NonInteractive: cfg.nonInteractive,
		Verbose: cfg.verbosity >= VerbosityVerbose,
	})
	o.finalPlain = string(fullPlain)

	// Human stream: only residual (terminal outcomes already streamed).
	residual := o.residualPlainLocked(snap)
	interactive := false
	if live := o.liveLocked(); live != nil && live.IsInteractive() && !cfg.plain {
		interactive = true
		// Interactive final: conclusion + any unemitted entities (not a second full dump).
		o.finishLiveLocked(o.residualInteractiveFinalLocked(snap))
	}
	o.closeSnapshotsLocked()
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
	} else if writer != nil {
		// Dual stream: residual conclusion on primary (items already durable on terminal).
		if _, err := io.WriteString(writer, residual); err != nil {
			writeErr = fmt.Errorf("%w: %v", ErrRenderer, err)
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

// FinalPlain returns the last rendered plain text after Finish.
func (o *Output) FinalPlain() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.finalPlain
}
