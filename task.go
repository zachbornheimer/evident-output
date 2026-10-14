package evo

import (
	"fmt"
	"os"

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
	// state is the render state, guarded by o.mu; nil for a handle rebuilt from an id.
	state *taskState
	// rejected is why the declaration was refused; nil for a declared Task.
	rejected error
}

func (t *Task) declared() *taskState {
	if t.state != nil {
		return t.state
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskStates[t.id]
}

func (t *Task) succeed(summary string) {
	t.finish(succeeded, project.Text(summary), nil)
}

func (t *Task) resolveWithProblem(state entityState, summary string, options []ProblemOption) {
	p := applyProblemOptions(project.Text(summary), options)
	t.finish(state, project.Text(summary), []problem{p})
}

func (t *Task) block(summary string, options ...ProblemOption) {
	t.resolveWithProblem(refused, summary, options)
}

func (t *Task) cancel(reason string) {
	t.finish(cancelled, project.Text(reason), nil)
}

// finish is the resolution path behind every terminal verb. Once a task is
// submitted, a requested success is only a proposal the callback's return ratifies.
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

// Predecessor is a handle After accepts: *Task, *Group, *Sequence or *Computed[T]. Sealed.
type Predecessor interface {
	predecessor()
}

func (*Task) predecessor()     {}
func (*Group) predecessor()    {}
func (*Sequence) predecessor() {}

// After declares predecessors: this Task starts only once all succeeded, never if one cannot.
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

func (o *Run) predecessorsOfLocked(args []Predecessor) []graph.Predecessor {
	preds := make([]graph.Predecessor, 0, len(args))
	for _, arg := range args {
		if pred, ok := o.predecessorOfLocked(arg); ok {
			preds = append(preds, pred)
		}
	}
	return preds
}

// predecessorOfLocked resolves one After argument; a handle from another Output yields the empty predecessor, which never succeeds.
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

func (o *Run) nodeOfTask(id string) *graph.Task {
	if st := o.taskStates[id]; st != nil {
		return st.node
	}
	return nil
}

func (o *Run) nodeOfContainer(id string) *graph.Container {
	if st := o.containerStates[id]; st != nil {
		return st.node
	}
	return nil
}

// After declares predecessors: this Group declares children only once all succeeded.
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

// define defers this Group's children: build runs once all After predecessors succeeded,
// and is topology only (no context, no Wait). If one fails, build never runs; the Group settles NotStarted.
func (g *Group) define(build func(*Group)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

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

// Task declares a single root-level operation named name. A repeated name is a
// duplicate sibling declaration (§3.1), never a get-or-create.
func (o *Run) Task(name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskHandle(o.graph.DeclareTask(nil, name))
}

// Group declares independent child tasks that may overlap; a repeated name is a duplicate (§3.1).
func (o *Run) Group(name string) *Group {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.groupHandle(o.graph.DeclareContainer(nil, name, false))
}

// Sequence declares an ordered container: once a member fails or is cancelled,
// later members never start. A repeated name is a duplicate sibling declaration (§3.1).
func (o *Run) Sequence(name string) *Sequence {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &Sequence{tasks: o.groupHandle(o.graph.DeclareContainer(nil, name, true))}
}

// taskHandle is the declared Task, or a refused handle keeping err. Callers hold o.mu.
func (o *Run) taskHandle(node *graph.Task, err error) *Task {
	if err != nil {
		return o.rejectedTask(err)
	}
	return o.taskStates[node.ID].handle
}

func (o *Run) groupHandle(node *graph.Container, err error) *Group {
	if err != nil {
		return o.rejectedGroup(err)
	}
	return o.containerStates[node.ID].handle
}

// Container is a node that can declare work beneath it: the run, a Group or a Sequence.
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

// Group is the front door for independent child work; eligible children may overlap.
type Group struct {
	out *Run
	id  string
	// rejected is why the declaration was refused; nil for a declared Group or Sequence.
	rejected error
}

// Task declares a child task. A repeated name fails with ProblemCodeDuplicateSiblingName (§3.1),
// so keep the first handle if you reference the Task again (for example in After).
func (g *Group) Task(name string) *Task {
	if g == nil || g.out == nil {
		return &Task{}
	}
	return g.out.declareChildTask(g, name)
}

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

// Sequence is the front door for ordered child work: Group's scheduler with implicit
// predecessor edges. A failed, blocked or cancelled child makes later siblings NotStarted.
type Sequence struct {
	tasks *Group
}

// Task declares a child Task that runs after the Sequence's earlier children settle. Sibling names are unique
// per parent: declaring a name the Sequence already holds is misuse and returns a Task that never runs.
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

// Doing sets the live step text and starts the task if pending; text is a printf format when
// args are present. Chained right after Task, it sets the first step. Off a TTY each call is a durable line.
func (t *Task) Doing(text string, args ...any) *Task {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
	return t.annotate(func(st *taskState) { t.out.setPhaseLocked(st, text) })
}

// resolvedByInterrupt: narrating a row the interrupt sweep resolved is not misuse; a worker mid-step cannot stop a straggling update.
func resolvedByInterrupt(state entityState) bool {
	return state == cancelled || state == notStarted
}

// annotate guards every non-terminal annotation verb: under o.mu it applies only while the task is open.
// A closed Output records misuse; a terminal row records ErrAlreadyResolved unless the interrupt sweep resolved it.
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

// withTask is the one lock-and-lookup for row-editing verbs: under o.mu it hands apply the task's
// state and does nothing for a nil, zero or unknown Task, so "a nil *Task is safe" holds for every verb.
func (t *Task) withTask(apply func(st *taskState)) *Task {
	if t == nil || t.out == nil {
		return t
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	if st := t.out.taskStates[t.id]; st != nil {
		apply(st)
		t.out.show.Board.Changed(st.node.TaskID())
	}
	return t
}

// setLiveOnlyPhase sets phase text for non-narrated sources such as Writer's per-line mirror;
// unlike Doing it never forces a durable row off-TTY (the evidence ring holds the output).
func (t *Task) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

func (o *Run) setPhaseLocked(st *taskState, text string) {
	if st.node.Doing(text, o.cfg.clock.Now()) {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.show.Live.Signal(true)
	o.show.Stream.Running(st.node.TaskID(), renderplain.TriggerPhase, o.cfg.clock.Now())
}

// setLiveOnlyPhaseLocked is setPhaseLocked without the forced plain-mode durable line.
func (o *Run) setLiveOnlyPhaseLocked(st *taskState, text string) {
	// An identical live-only phase is a no-op: repeated Writer lines must not bump, emit or repaint.
	changed, started := st.node.Mirror(text, o.cfg.clock.Now())
	if !changed {
		return
	}
	if started {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.show.Live.Signal(true)
}

func (t *Task) recordWarning(p problem) *Task {
	return t.annotate(func(st *taskState) {
		if st.node.Warn(p) {
			t.out.show.Board.Warned(st.node.TaskID())
		}
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.warned", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventWarningRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.show.Live.Signal(true)
	})
}

// Summary sets one sanitized line of result text for the terminal row; last call wins, empty clears.
// It never resolves the task. After the task resolved it is misuse unless the interrupt sweep resolved it.
func (t *Task) Summary(text string) *Task {
	return t.annotate(func(st *taskState) {
		st.node.Summarize(text)
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.summary_set", EntityID: t.id})
		t.out.show.Live.Signal(true)
	})
}

// Problem attaches one structured diagnostic. It never ends the Task or changes its outcome, which settles
// from the Define callback's return alone; return a fatal finding through Fail. An invalid severity is rejected as misuse.
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
		t.out.show.Live.Signal(true)
	})
}

// Fact accumulates a name/value annotation (info severity), the non-terminal sibling of Problem.
// Renders as a dim "name  value" line, inline when it is the only annotation.
func (t *Task) Fact(name, value string) *Task {
	f := record.SanitizeFact(factRecord{Name: project.Text(name), Value: project.Text(value)})
	return t.annotate(func(st *taskState) {
		st.node.Note(f)
		t.out.bumpLocked()
		t.out.emitWireEventLocked(wire.EventFactRecorded, t.id, wire.ToFactDoc(f).EventPayload())
		t.out.show.Live.Signal(true)
	})
}

func (t *Task) appendLiveTail(line string, retained int) {
	t.annotate(func(st *taskState) {
		line := project.Text(line)
		t.out.show.Board.PushTail(st.node.TaskID(), line, retained, t.out.cfg.clock.Now())
		t.out.setLiveOnlyPhaseLocked(st, line)
	})
}

type DefineOption func(*defineConfig)

type defineConfig struct{ cleanStopSignals []os.Signal }

func CleanStop(signals ...os.Signal) DefineOption {
	return func(c *defineConfig) { c.cleanStopSignals = append(c.cleanStopSignals, signals...) }
}

func (g *Group) Build(fn func(*Group)) { g.define(fn) }

func (s *Sequence) Build(fn func(*Sequence)) { s.define(fn) }
