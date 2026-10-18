package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
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

// Doing sets the live step text and starts the task if pending; text is a printf format when
// args are present. Chained right after Task, it sets the first step. Off a TTY the first call is a durable
// line and later calls thin into milestones carrying the latest text; the heartbeat row covers the silence.
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
// unlike Doing, repeating the current text is a no-op (the evidence ring holds the output).
func (t *Task) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

func (r *Run) setPhaseLocked(st *taskState, text string) {
	st.node.Doing(text, r.cfg.clock.Now())
	r.bumpLocked()
	r.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	r.show.Live.Signal(true)
	r.show.Stream.Narrated(st.node.TaskID(), r.cfg.clock.Now())
}

// setLiveOnlyPhaseLocked is setPhaseLocked for non-narrated sources.
func (r *Run) setLiveOnlyPhaseLocked(st *taskState, text string) {
	// An identical live-only phase is a no-op: repeated Writer lines must not bump, emit or repaint.
	if !st.node.Mirror(text, r.cfg.clock.Now()) {
		return
	}
	r.bumpLocked()
	r.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	r.show.Live.Signal(true)
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
// from the Define callback's return alone; return a fatal finding through Fail.
func (t *Task) Problem(summary string, opts ...ProblemOption) *Task {
	p := applyProblemOptions(project.Text(summary), opts)
	return t.annotate(func(st *taskState) {
		st.node.Report(p)
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.problem_recorded", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventProblemRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.show.Live.Signal(true)
	})
}

// Fact accumulates a name/value annotation (information, not a verdict), the non-terminal sibling of Problem.
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
