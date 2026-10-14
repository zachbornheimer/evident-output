package evo

import (
	"context"
	"fmt"
	"os"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	renderplain "github.com/zachbornheimer/evident-output/v2/internal/project/plain"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
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

// Verify registers a read-only check of the world's current state, ANDed with any earlier Verify on this Task.
// Call it before Define, which freezes configuration; a later call is ignored and recorded as misuse.
//
// Evo runs every Verify twice. Before the callback, all true skips it and settles the Task Satisfied, any
// false runs it, and an error fails the Task. After a callback that returned nil, any false fails the Task,
// because the work did not produce the state it promised. The second check is skipped when the callback
// ended the Task with an error, or when a dry run planned its changes without making them.
func (t *Task) Verify(fn func(context.Context) (bool, error)) *Task {
	if t == nil || t.out == nil || fn == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	if st.node.Submitted() {
		o.recordMisuseFor(st.name, ErrInvalidConfig)
		return t
	}
	o.freshness.Tasks().AddVerifier(st.node.TaskID(), freshness.Verifier(fn))
	return t
}

// Define freezes this Task's After and Verify configuration and submits fn to the scheduler. It returns the
// same *Task at once and never runs fn inline; Evo decides when fn runs and owns its goroutine, so callers
// declare work and return. What fn returns is the Task's outcome: nil Succeeded, Refuse Refused, Exclude
// Excluded, and Fail or any other error Failed. A second Define on the same Task is misuse.
func (t *Task) Define(fn func(context.Context) error, opts ...DefineOption) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	if fn == nil {
		o.recordMisuse(ErrInvalidConfig)
		return t
	}
	st := t.declared()
	if st == nil {
		o.recordMisuse(t.rejected)
		return t
	}
	var cfg defineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	graph.Define(o.graph, st.node, o.definition(st, o.cleanStops.Wrap(cfg.cleanStopSignals, fn)))
	return t
}

// definition is st's Define with fn as its callback; freshness checks it.
func (o *Run) definition(st *taskState, fn func(context.Context) error) graph.Definition[*change.Run] {
	return graph.Definition[*change.Run]{Owner: o.changes, Checks: o.freshness.Checks(st.node.TaskID()), Callback: fn}
}
