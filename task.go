package evo

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/project"
	renderplain "github.com/zachbornheimer/evident-output/internal/project/plain"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Task is a handle for one operation with phases or progress.
type Task struct {
	out *Run
	id  string
	// state is the Task's render state, set once when the declaration made
	// this handle; nil for a handle rebuilt from an id (see declared). Its
	// fields stay guarded by o.mu.
	state *taskState
	// rejected is why the declaration was refused (see rejectedTask); nil
	// for a declared Task.
	rejected error
}

// declared is t's render state, nil for a refused declaration. The handle a
// declaration made carries it, so Define reaches it without o.mu.
func (t *Task) declared() *taskState {
	if t.state != nil {
		return t.state
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskStates[t.id]
}

// succeed resolves the task Done with summary: the engine's synchronous
// success verb for library-owned rows (Confirm's gate) and engine tests.
// The public Task has no equivalent since 1.1: callers resolve through
// Define, and Summary carries their result.
func (t *Task) succeed(summary string) {
	t.finish(succeeded, project.Text(summary), nil)
}

// resolveWithProblem is Fail and Block: resolve as state with one Problem
// built from summary and options.
func (t *Task) resolveWithProblem(state entityState, summary string, options []ProblemOption) {
	p := applyProblemOptions(project.Text(summary), options)
	t.finish(state, project.Text(summary), []problem{p})
}

// block resolves the task as refused. A nil *Task is safe and resolves
// nothing.
func (t *Task) block(summary string, options ...ProblemOption) {
	t.resolveWithProblem(refused, summary, options)
}

// cancel resolves the task as cancelled. A nil *Task is safe and resolves
// nothing.
func (t *Task) cancel(reason string) {
	t.finish(cancelled, project.Text(reason), nil)
}

// finish is the caller-facing resolution path behind every terminal verb a
// program writes by hand; see graph.Graph.Resolve and graph.Graph.Admit.
// Once a task is submitted, a success the caller asks for is a proposal the
// callback's return ratifies; bad news resolves at once.
func (t *Task) finish(state entityState, summary string, problems []problem) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	o.mu.Unlock()
	if st != nil {
		o.graph.Resolve(st.node, graph.Resolution{State: state, Summary: summary, Problems: problems, By: graph.ByCaller})
	}
	return t
}

// Predecessor is a handle After accepts: a *Task, *Group, *Sequence or
// *Computed[T]. It is sealed, so nothing else can be passed to After.
type Predecessor interface {
	predecessor()
}

func (*Task) predecessor()     {}
func (*Group) predecessor()    {}
func (*Sequence) predecessor() {}

// After declares predecessors: this Task starts only once every one of
// them succeeded, and never starts once one of them cannot.
func (t *Task) After(preds ...Predecessor) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	o.graph.After(st.node, o.predecessorsOfLocked(preds)...)
	return t
}

// predecessorsOfLocked resolves After's arguments, leaving out the ones
// After ignores.
func (o *Run) predecessorsOfLocked(args []Predecessor) []graph.Predecessor {
	preds := make([]graph.Predecessor, 0, len(args))
	for _, arg := range args {
		if pred, ok := o.predecessorOfLocked(arg); ok {
			preds = append(preds, pred)
		}
	}
	return preds
}

// predecessorOfLocked resolves one After argument. ok is false for a nil
// handle, which After ignores. A handle this Output never declared yields
// the empty predecessor, which never succeeds.
func (o *Run) predecessorOfLocked(p Predecessor) (pred graph.Predecessor, ok bool) {
	switch x := p.(type) {
	case *Task:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterTask(o.nodeOfTask(x.id)), true
		}
	case *Group:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterContainer(o.nodeOfContainer(x.id)), true
		}
	case *Sequence:
		if x == nil || x.tasks == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.tasks)
	case interface{ producer() *Task }:
		if x == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.producer())
	default:
		return graph.Predecessor{}, false
	}
	return graph.Predecessor{}, true
}

// nodeOfTask is the graph's declaration of the Task named id, nil when this
// Output declared none.
func (o *Run) nodeOfTask(id string) *graph.Task {
	if st := o.taskStates[id]; st != nil {
		return st.node
	}
	return nil
}

// nodeOfContainer is the graph's declaration of the container named id, nil
// when this Output declared none.
func (o *Run) nodeOfContainer(id string) *graph.Container {
	if st := o.containerStates[id]; st != nil {
		return st.node
	}
	return nil
}

// After declares predecessors: this Group declares its children only once
// every one of them succeeded, and never does once one of them cannot. A
// predecessor may be a Task, a Computed producer, or a Group or Sequence.
// Call it before Define and before declaring any child; later is misuse.
func (g *Group) After(preds ...Predecessor) *Group {
	if g == nil || g.out == nil {
		return g
	}
	o := g.out
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return g
	}
	o.graph.ContainerAfter(col.node, o.predecessorsOfLocked(preds)...)
	return g
}

// After is Group.After for a Sequence.
func (s *Sequence) After(preds ...Predecessor) *Sequence {
	if s == nil || s.tasks == nil {
		return s
	}
	s.tasks.After(preds...)
	return s
}

// Define defers this Group's children: build runs once, when every After
// predecessor succeeded, and declares the children through the handle it
// receives. build is topology only. It returns nothing, takes no context,
// and must not Wait. Children it declares run under the Group's normal
// semantics. When a predecessor fails, build never runs and the Group
// settles NotStarted.
func (g *Group) define(build func(*Group)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

// Define is Group.Define for a Sequence: build declares ordered steps.
func (s *Sequence) define(build func(*Sequence)) {
	if s == nil || s.tasks == nil {
		return
	}
	if build == nil {
		s.tasks.recordInvalid()
		return
	}
	s.tasks.defineBuilder(func() { build(s) })
}

func (g *Group) recordInvalid() {
	if g != nil && g.out != nil {
		g.out.recordMisuse(errInvalidConfig)
	}
}

func (g *Group) defineBuilder(run func()) {
	if g == nil || g.out == nil {
		return
	}
	o := g.out
	o.mu.Lock()
	col := o.containerStates[g.id]
	o.mu.Unlock()
	if col == nil {
		o.recordMisuse(g.rejected)
		return
	}
	o.graph.DefineBuilder(col.node, run)
}

// Task declares a single root-level operation named name. A repeated name
// is a duplicate sibling declaration (§3.1), never a get-or-create: see
// graph.Graph.DeclareTask.
func (o *Run) Task(name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskHandle(o.graph.DeclareTask(nil, name))
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create.
func (o *Run) Group(name string) *Group {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.groupHandle(o.graph.DeclareContainer(nil, name, false))
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated name is a
// duplicate sibling declaration (§3.1), not a get-or-create.
func (o *Run) Sequence(name string) *Sequence {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &Sequence{tasks: o.groupHandle(o.graph.DeclareContainer(nil, name, true))}
}

// taskHandle is the handle of the Task the graph declared, or a refused
// handle keeping err. Callers hold o.mu.
func (o *Run) taskHandle(node *graph.Task, err error) *Task {
	if err != nil {
		return o.rejectedTask(err)
	}
	return o.taskStates[node.ID].handle
}

// groupHandle is taskHandle for a Group or Sequence. Callers hold o.mu.
func (o *Run) groupHandle(node *graph.Container, err error) *Group {
	if err != nil {
		return o.rejectedGroup(err)
	}
	return o.containerStates[node.ID].handle
}

// Container is a node that can declare work beneath it: the run, a Group,
// or a Sequence. A Task is never a Container.
//
// Shared topology builders take a Container:
//
//	func Build(parent evo.Container)
type Container interface {
	Task(name string) *Task
	Group(name string) *Group
	Sequence(name string) *Sequence
}

var (
	_ Container = (*Run)(nil)
	_ Container = (*Group)(nil)
	_ Container = (*Sequence)(nil)
)

// Group is the front door for independent child work. Eligible
// children may overlap through the scheduler. The type cannot be named
// Group — that name is the constructor. Nest via Group/Sequence.
type Group struct {
	out *Run
	id  string
	// rejected is why the declaration was refused (see rejectedGroup); nil
	// for a declared Group or Sequence.
	rejected error
}

// Task declares a child task. A repeated name is a duplicate sibling
// declaration, not a get-or-create (§3.1, Output.Task's doc comment): it
// fails the new call with ProblemCodeDuplicateSiblingName rather than
// returning the earlier handle. Callers that reference a Task again later
// (for example in After) must keep the first handle, typically in a typed
// variable, instead of re-declaring by name.
func (g *Group) Task(name string) *Task {
	if g == nil || g.out == nil {
		return &Task{}
	}
	return g.out.declareChildTask(g, name)
}

// declareChildTask declares a Task named name in the container g. A Task
// declared under a refused container is refused for the same reason.
func (o *Run) declareChildTask(g *Group, name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	return o.taskHandle(o.graph.DeclareTask(col.node, name))
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *Group) Sequence(name string) *Sequence {
	child := g.declareChild(name, true)
	return &Sequence{tasks: child}
}

// Group declares (or returns) an independent child container nested here.
func (g *Group) Group(name string) *Group {
	return g.declareChild(name, false)
}

func (g *Group) declareChild(name string, sequential bool) *Group {
	if g == nil || g.out == nil {
		return &Group{}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	parent := g.out.containerStates[g.id]
	if parent == nil {
		return g.out.rejectedGroup(g.rejected)
	}
	return g.out.groupHandle(g.out.graph.DeclareContainer(parent.node, name, sequential))
}

// Summary sets a success-oriented collection summary.
func (g *Group) Summary(text string) *Group {
	if g == nil || g.out == nil {
		return g
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.containerStates[g.id]
	if col == nil {
		return g
	}
	if err := g.out.ensureOpen(); err != nil {
		g.out.recordMisuse(err)
		return g
	}
	col.node.Summarize(text)
	g.out.bumpLocked()
	g.out.appendEventLocked(recEvent{Type: "tasks.summary_set", EntityID: g.id})
	return g
}

// Sequence is the front door for ordered child work: the same
// scheduler as Group with implicit predecessor edges in declaration order.
// A failed/blocked/cancelled child makes later siblings NotStarted.
type Sequence struct {
	tasks *Group
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order.
func (g *Sequence) Task(name string) *Task {
	if g == nil || g.tasks == nil {
		return &Task{}
	}
	return g.tasks.out.declareChildTask(g.tasks, name)
}

// Summary sets a success-oriented sequence summary.
func (g *Sequence) Summary(text string) *Sequence {
	if g == nil || g.tasks == nil {
		return g
	}
	g.tasks.Summary(text)
	return g
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *Sequence) Sequence(name string) *Sequence {
	if g == nil || g.tasks == nil {
		return &Sequence{}
	}
	return g.tasks.Sequence(name)
}

// Group declares (or returns) an independent child container nested here.
func (g *Sequence) Group(name string) *Group {
	if g == nil || g.tasks == nil {
		return &Group{}
	}
	return g.tasks.Group(name)
}

// Doing sets the active current-step live text and starts the task if
// pending — replaces the previous text, promotes the task to Running, and
// becomes a durable line per step off-TTY. text is a printf format when args
// are present (fmt.Sprintf semantics). Chained right after Task, it sets
// the first step at declaration.
func (t *Task) Doing(text string, args ...any) *Task {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
	return t.annotate(func(st *taskState) { t.out.setPhaseLocked(st, text) })
}

// resolvedByInterrupt reports whether this state was reached by the
// interrupt sweep rather than by the caller. Narrating such a row is not
// misuse: cancellation resolves it underneath whoever was reporting on it,
// and a worker already inside its per-item step had no way to prevent the
// one straggling update that follows. Blaming the caller for the interrupt's
// own timing put a misuse warning at the top of every interrupted ledger.
func resolvedByInterrupt(state entityState) bool {
	return state == cancelled || state == notStarted
}

// annotate is the one guard every non-terminal annotation verb (Doing,
// Progress, Bytes, Summary, Problem, Fact) shares: under o.mu,
// it applies apply to the task's state only while the task is open. On a
// closed Output it records that misuse; on a terminal row it records
// ErrAlreadyResolved, unless the interrupt sweep resolved the row (see
// resolvedByInterrupt). It returns t so each verb can chain.
func (t *Task) annotate(apply func(st *taskState)) *Task {
	return t.withTask(func(st *taskState) {
		if err := t.out.ensureOpen(); err != nil {
			t.out.recordMisuse(err)
			return
		}
		if record.IsTerminalTask(st.node.Rec.State()) {
			if !resolvedByInterrupt(st.node.Rec.State()) {
				t.out.recordMisuseFor(st.name, errAlreadyResolved)
			}
			return
		}
		apply(st)
	})
}

// withTask is the one lock-and-lookup every Task verb that
// edits its row goes through: under o.mu it hands apply the task's state,
// and does nothing for a nil or zero Task or a task this Output does
// not know. That is where "a nil *Task is safe" is kept, for every
// verb at once. It returns t so each verb can chain.
func (t *Task) withTask(apply func(st *taskState)) *Task {
	if t == nil || t.out == nil {
		return t
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	if st := t.out.taskStates[t.id]; st != nil {
		apply(st)
		t.out.board.Changed(st.node.TaskID())
	}
	return t
}

// setLiveOnlyPhase updates the task's phase text through setLiveOnlyPhaseLocked
// — the shared entry point for every phase source that is NOT the caller's
// own narrated beat: Writer's per-line mirror of a talkative child's raw
// output. Off-TTY, an explicit Task.Doing call
// still forces its own durable row (the P10 contract: the one line the
// caller asked to see); this path never does — a child's full output already
// has one durable home, the evidence ring (and its failure-path DetailTail),
// so a row per mirrored line would just repeat it (release-gate round 9
// finding 4).
func (t *Task) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

// setPhaseLocked is Doing's locked body. Callers must already hold o.mu and
// have checked ensureOpen/isTerminalTask.
func (o *Run) setPhaseLocked(st *taskState, text string) {
	if st.node.Doing(text, o.cfg.clock.Now()) {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.live.Signal(true)
	o.stream.Running(st.node.TaskID(), renderplain.TriggerPhase, o.cfg.clock.Now())
}

// setLiveOnlyPhaseLocked is the shared locked body behind every phase
// source that is not the caller's own narrated beat (see setLiveOnlyPhase):
// the same state transition as setPhaseLocked (promotion, activity clock,
// live redraw signal), but it never forces its own durable line in plain
// mode. A talkative child's mirrored output line (Writer) already has one
// durable home, the evidence ring, so it gets no plain-mode row per line
// (release-gate round 9 finding 4).
func (o *Run) setLiveOnlyPhaseLocked(st *taskState, text string) {
	// Identical live-only phase is a no-op: Writer leftover/repeated lines
	// must not bump, emit task.phase_changed, or force a live paint.
	changed, started := st.node.Mirror(text, o.cfg.clock.Now())
	if !changed {
		return
	}
	if started {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.live.Signal(true)
}

// recordWarning is the warning-severity Problem path: st.warnings,
// task.warned, EventWarningRecorded, the board's first warning. Public Warn was removed in 1.1.
func (t *Task) recordWarning(p problem) *Task {
	return t.annotate(func(st *taskState) {
		if st.node.Warn(p) {
			t.out.board.Warned(st.node.TaskID())
		}
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.warned", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventWarningRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.live.Signal(true)
	})
}

// Summary sets one sanitized line of result text for the task's terminal
// row: last call wins, empty clears it. It never resolves the task (Define
// or the Evo-native operation outcome does), and it is not live activity
// (Doing/Progress/Bytes own the Running row). It is the same field
// TaskSnapshot.Summary and JSON/JSONL "summary" project. Calling it after
// the task resolved is misuse unless the interrupt sweep resolved it (see
// annotate). Group.Summary is the same shape one level up.
func (t *Task) Summary(text string) *Task {
	return t.annotate(func(st *taskState) {
		st.node.Summarize(text)
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.summary_set", EntityID: t.id})
		t.out.live.Signal(true)
	})
}

// Problem attaches one structured diagnostic to the Task. It is an
// annotation: it never ends the Task or changes its outcome, which settles
// from the Define callback's return alone and carries the Problem in every
// output mode. A fatal finding is returned through Fail. SeverityWarning
// uses the warning projection. An invalid severity is rejected with a
// context-bearing misuse error and is not recorded.
func (t *Task) Problem(summary string, opts ...ProblemOption) *Task {
	p := applyProblemOptions(project.Text(summary), opts)
	sev, err := record.ClassifiedProblemSeverity(p)
	if err != nil {
		return t.annotate(func(st *taskState) { t.out.recordMisuse(err) })
	}
	if sev == severityWarning {
		return t.recordWarning(p)
	}
	return t.recordBlockingProblem(p)
}

func (t *Task) recordBlockingProblem(p problem) *Task {
	return t.annotate(func(st *taskState) {
		st.node.Report(p)
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.problem_recorded", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventProblemRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.live.Signal(true)
	})
}

// Fact accumulates a discovered name/value annotation on the task — info
// severity, Problem(SeverityWarning)'s non-terminal sibling (user-13-problems.md Problem 8:
// "Tasks are work. Facts are information."). Renders as a dim "name  value"
// line, inline when it is the task's only annotation, nested otherwise.
// Like Problem, it returns the Task for chaining and never resolves it — call
// it any number of times before the task's terminal verb.
func (t *Task) Fact(name, value string) *Task {
	f := record.SanitizeFact(factRecord{Name: project.Text(name), Value: project.Text(value)})
	return t.annotate(func(st *taskState) {
		st.node.Note(f)
		t.out.bumpLocked()
		t.out.emitWireEventLocked(wire.EventFactRecorded, t.id, wire.ToFactDoc(f).EventPayload())
		t.out.live.Signal(true)
	})
}

// appendLiveTail records one completed Writer line: it enters the live tail
// and becomes the phase, exactly as a mirrored line always has.
func (t *Task) appendLiveTail(line string, retained int) {
	t.annotate(func(st *taskState) {
		line := project.Text(line)
		t.out.board.PushTail(st.node.TaskID(), line, retained, t.out.cfg.clock.Now())
		t.out.setLiveOnlyPhaseLocked(st, line)
	})
}

// taskFinishedPayload is the task.finished event's payload: the Outcome
// the Task settled with, and its Summary when it has one (ZYS-971).
func taskFinishedPayload(st *taskState) map[string]any {
	payload := map[string]any{
		"outcome": string(st.node.Rec.State().Outcome()),
	}
	if st.node.Rec.Summary() != "" {
		payload["summary"] = st.node.Rec.Summary()
	}
	return payload
}

type DefineOption func(*defineConfig)

type defineConfig struct{ cleanStopSignals []os.Signal }

func CleanStop(signals ...os.Signal) DefineOption {
	return func(c *defineConfig) { c.cleanStopSignals = append(c.cleanStopSignals, signals...) }
}

func (g *Group) Build(fn func(*Group)) { g.define(fn) }

func (s *Sequence) Build(fn func(*Sequence)) { s.define(fn) }

// cleanStopTask is one running Task that declared CleanStop: the signals
// that end it Succeeded, and the cancel of its own context.
type cleanStopTask struct {
	signals []os.Signal
	cancel  context.CancelFunc
	stopped atomic.Bool
}

func (c *cleanStopTask) names(sig os.Signal) bool {
	return slices.Contains(c.signals, sig)
}

// cleanStopRegistry holds the CleanStop Tasks that are running now. Its zero
// value is ready to use.
type cleanStopRegistry struct {
	mu      sync.Mutex
	running map[*cleanStopTask]struct{}
}

func (r *cleanStopRegistry) add(c *cleanStopTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running == nil {
		r.running = map[*cleanStopTask]struct{}{}
	}
	r.running[c] = struct{}{}
}

func (r *cleanStopRegistry) remove(c *cleanStopTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, c)
}

// stop cancels every running Task that names sig and reports whether any
// did; when none does, the signal belongs to the run.
func (r *cleanStopRegistry) stop(sig os.Signal) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	handled := false
	for c := range r.running {
		if c.names(sig) {
			c.stopped.Store(true)
			c.cancel()
			handled = true
		}
	}
	return handled
}

// wrap runs fn under a context a named signal can cancel, and reports a
// callback stopped that way as success.
func (r *cleanStopRegistry) wrap(signals []os.Signal, fn func(context.Context) error) func(context.Context) error {
	if len(signals) == 0 {
		return fn
	}
	return func(ctx context.Context) error {
		taskCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		task := &cleanStopTask{signals: signals, cancel: cancel}
		r.add(task)
		defer r.remove(task)
		err := fn(taskCtx)
		if task.stopped.Load() {
			return nil
		}
		return err
	}
}
