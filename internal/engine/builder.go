package engine

import (
	"errors"
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// After declares predecessors: this Group declares its children only once
// every one of them succeeded, and never does once one of them cannot. A
// predecessor may be a Task, a Computed producer, or a Group or Sequence.
// Call it before Define and before declaring any child; later is misuse.
func (g *GroupHandle) After(preds ...any) *GroupHandle {
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
	if !o.graph.AddContainerAfter(col.node, o.predecessorsOfLocked(preds)...) {
		o.recordMisuse(ErrInvalidConfig)
	}
	return g
}

// After is GroupHandle.After for a Sequence.
func (s *SequenceHandle) After(preds ...any) *SequenceHandle {
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
func (g *GroupHandle) Define(build func(*GroupHandle)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

// Define is GroupHandle.Define for a Sequence: build declares ordered steps.
func (s *SequenceHandle) Define(build func(*SequenceHandle)) {
	if s == nil || s.tasks == nil {
		return
	}
	if build == nil {
		s.tasks.recordInvalid()
		return
	}
	s.tasks.defineBuilder(func() { build(s) })
}

func (g *GroupHandle) recordInvalid() {
	if g != nil && g.out != nil {
		g.out.recordMisuse(ErrInvalidConfig)
	}
}

func (g *GroupHandle) defineBuilder(run func()) {
	if g == nil || g.out == nil {
		return
	}
	o := g.out
	o.mu.Lock()
	col := o.containerStates[g.id]
	switch {
	case col == nil:
		o.recordMisuse(g.rejected)
		o.mu.Unlock()
		return
	case o.ensureOpen() != nil:
		o.recordMisuse(ErrClosed)
		o.mu.Unlock()
		return
	}
	node := o.graph.AddGate(col.node, func() error { run(); return nil })
	if node == nil {
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	gate := &taskState{id: node.ID, node: node, name: node.Name, rec: node.Rec, followed: declaredState, declaration: node.Declaration}
	gate.handle = &TaskHandle{out: o, id: gate.id}
	o.taskStates[gate.id] = gate
	o.graph.Enqueue(node, nil)
	if o.sched.cancelled {
		o.graph.MarkNotStarted(node)
		o.mu.Unlock()
		return
	}
	o.graph.Place(node)
	o.mu.Unlock()
	o.kick()
}

// runGate runs a claimed container builder and settles its gate. The
// builder runs under builderFrames, not runCallback, so the declarations it
// makes are not Task-callback declarations (see declaredInCallback).
func (o *Output) runGate(st *taskState) {
	panicText := runBuilder(st.node.Work())
	o.mu.Lock()
	defer o.mu.Unlock()
	if panicText != "" {
		st.workErr = fmt.Errorf("declaring %s: panic: %s", st.name, panicText)
		o.settleLocked(st, Failed)
		return
	}
	o.settleLocked(st, Done)
}

// builderFrames marks runBuilder (see frameMarker).
var builderFrames graph.FrameMarker

func runBuilder(work func() error) (panicText string) {
	defer func() {
		if r := recover(); r != nil {
			panicText = fmt.Sprint(r)
		}
	}()
	builderFrames.Note()
	_ = work()
	return ""
}

// awaitBuilders parks until every topology builder under the container has
// settled, and returns why a builder did not declare its children. Nested
// builders are declared by their parent's, so each level is awaited after
// the one above it.
func (o *Output) awaitBuilders(id string, stack *waiterStack) error {
	o.mu.Lock()
	col := o.containerStates[id]
	o.mu.Unlock()
	if col == nil {
		return nil
	}
	return o.awaitBuildersIn(col, stack, &graph.InputSeals{})
}

func (o *Output) awaitBuildersIn(col *tasksState, stack *waiterStack, seen *graph.InputSeals) error {
	var errs []error
	if gate := o.gateHandle(col); gate != nil {
		if err := gate.waitChecked(stack, seen); err != nil {
			errs = append(errs, err)
		}
	}
	o.mu.Lock()
	children := slices.Clone(col.children)
	o.mu.Unlock()
	for _, child := range children {
		if err := o.awaitBuildersIn(child, stack, seen); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// gateHandle is the handle of col's builder gate, nil when col has none.
func (o *Output) gateHandle(col *tasksState) *TaskHandle {
	node := col.node.BuilderGate()
	if node == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskStates[node.ID].handle
}

// withBuilderOutcome folds a container Wait's builder outcome into its
// descendants' outcome. A builder that never ran is derivative when some
// descendant already answers, and the whole answer otherwise, so a Group
// that declared nothing because its predecessor failed does not wait to nil.
func withBuilderOutcome(builderErr, descendantsErr error) error {
	switch {
	case builderErr == nil:
		return descendantsErr
	case descendantsErr == nil:
		return builderErr
	case errors.Is(builderErr, ErrNotStarted):
		return descendantsErr
	default:
		return errors.Join(builderErr, descendantsErr)
	}
}

// declaredInCallback reports whether the calling goroutine is inside a Task
// callback that is not running a topology builder. Declaring from a
// callback is misuse: declare it from a builder, or before the run.
func (o *Output) declaredInCallback() bool {
	if o.sched.executing == 0 {
		return false
	}
	m := readStackMarks()
	return m.callbacks > m.builders
}
