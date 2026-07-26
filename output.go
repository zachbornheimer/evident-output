package evo

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

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
}

type itemState struct {
	id          string
	name        string
	state       EntityState
	problems    []Problem
	because     string
	actions     []Action
	declaration int
	handle      *Item
}

type taskState struct {
	id          string
	name        string
	state       EntityState
	phase       string
	progress    Progress
	summary     string
	problems    []Problem
	actions     []Action
	collection  *tasksState
	declaration int
	handle      *Task
}

type tasksState struct {
	id          string
	name        string
	summary     string
	tasks       []*taskState
	declaration int
	handle      *Tasks
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

// New creates an Output without a primary subject.
func New(options ...Option) *Output {
	return newOutput("", options...)
}

// For creates an Output for a subject.
func For(subject string, options ...Option) *Output {
	return newOutput(subject, options...)
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
	}
	for _, opt := range options {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	if cfg.plain || cfg.nonInteractive {
		// interactive terminal ignored for plain path in v0.1
	}
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
	return o
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

// Item declares a named final-report condition.
func (o *Output) Item(name string) *Item {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &Item{out: o, id: o.nextID("item")}
	}
	st := &itemState{
		id:          o.nextID("item"),
		name:        sanitize.Text(name),
		state:       Pending,
		declaration: o.nextDecl(),
	}
	h := &Item{out: o, id: st.id}
	st.handle = h
	o.items = append(o.items, st)
	o.itemByRef[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "item.declared", EntityID: st.id})
	return h
}

// Task declares a single operation.
func (o *Output) Task(name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.addTaskLocked(sanitize.Text(name), nil)
}

func (o *Output) addTaskLocked(name string, col *tasksState) *Task {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &Task{out: o, id: o.nextID("task")}
	}
	st := &taskState{
		id:          o.nextID("task"),
		name:        name,
		state:       Pending,
		progress:    Progress{Kind: Indeterminate},
		collection:  col,
		declaration: o.nextDecl(),
	}
	h := &Task{out: o, id: st.id}
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

// Line emits a durable user-facing line.
func (o *Output) Line(message string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.lines = append(o.lines, sanitize.Text(message))
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "output.line_emitted"})
}

// Linef formats and emits a durable user-facing line.
func (o *Output) Linef(format string, args ...any) {
	o.Line(fmt.Sprintf(format, args...))
}

// Info emits an informational durable line.
func (o *Output) Info(message string, _ ...Field) {
	o.Line(message)
}

// WarnMessage emits a warning durable line (not an item warning).
func (o *Output) WarnMessage(message string, _ ...Field) {
	o.Line(message)
}

// ErrorMessage emits an error durable line (not an item failure).
func (o *Output) ErrorMessage(message string, _ ...Field) {
	o.Line(message)
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

// Debug records a diagnostic line; during live UI it is inserted above the region.
func (o *Output) Debug(message string, fields ...Field) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if o.cfg.debugLevel > LevelDebug {
		return
	}
	line := formatDebug(message, fields)
	o.lines = append(o.lines, line)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "log.emitted"})
	if o.liveLocked() != nil {
		o.debugLiveLocked(line)
	}
}

func formatDebug(message string, fields []Field) string {
	msg := sanitize.Text(message)
	if len(fields) == 0 {
		return "[DEBUG] " + msg
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		val := fmt.Sprint(f.Value)
		if f.Sensitive {
			val = "***"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", sanitize.Text(f.Key), sanitize.Text(val)))
	}
	return "[DEBUG] " + msg + "  " + joinArgs(parts)
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
	if o.conclusion != nil {
		c := *o.conclusion
		s.Conclusion = &c
	}
	return s
}

func (it *itemState) snapshot() ItemSnapshot {
	return ItemSnapshot{
		ID:          it.id,
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
		Name:        t.name,
		State:       t.state,
		Phase:       t.phase,
		Progress:    t.progress,
		Summary:     t.summary,
		Problems:    cloneProblems(t.problems),
		Actions:     cloneActions(t.actions),
		Collection:  colID,
		Declaration: t.declaration,
	}
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
	e.Sequence = uint64(len(o.events) + 1)
	e.Timestamp = o.cfg.clock.Now()
	e.SchemaVersion = EventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = o.outputID
	}
	o.events = append(o.events, e)
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
	interactiveFinal := renderInteractiveFinal(snap)
	if live := o.liveLocked(); live != nil && live.IsInteractive() {
		o.finishLiveLocked(interactiveFinal)
	}
	o.closeSnapshotsLocked()
	o.mu.Unlock()

	// Projection outside lock.
	plain, _ := RenderPlain(snap, PlainOptions{
		Width: cfg.width, NoColor: cfg.noColor, NonInteractive: cfg.nonInteractive,
	})
	o.mu.Lock()
	o.finalPlain = string(plain)
	o.mu.Unlock()
	if writer != nil {
		_, _ = writer.Write(plain)
	}
	return misuse
}

func renderInteractiveFinal(s Snapshot) string {
	var b strings.Builder
	for _, t := range s.Tasks {
		writeTask(&b, t)
	}
	for _, col := range s.Collections {
		writeCollection(&b, col)
	}
	for _, it := range s.Items {
		writeItem(&b, it)
	}
	if b.Len() == 0 {
		// fall back to full plain without conclusion
		cfg := config{width: defaultWidth, plain: true, nonInteractive: true}
		text := renderPlain(s, cfg)
		// strip trailing conclusion block if present
		return strings.TrimRight(text, "\n")
	}
	return strings.TrimRight(b.String(), "\n")
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
	return inferConclusion(snap)
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
