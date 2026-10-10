package engine

import (
	"errors"
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// builderPhase is where a container's topology builder stands.
type builderPhase uint8

const (
	// builderPending: Defined, and not finished: waiting on the
	// container's predecessors, or running.
	builderPending builderPhase = iota
	// builderDone: the builder returned; its children were declared.
	builderDone
	// builderNotStarted: a predecessor can never succeed, so the builder
	// never ran.
	builderNotStarted
	// builderFailed: the builder panicked before declaring every child.
	builderFailed
)

// containerBuilder is the declaration work a Group or Sequence deferred
// until its predecessors succeed (GroupHandle.Define). gate is the
// scheduler's entity for that work: it waits on the container's After
// edges like a Task does, but it is no row, and no collection counts it.
type containerBuilder struct {
	gate  *taskState
	phase builderPhase
}

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
	col := o.tasksByRef[g.id]
	if col == nil {
		return g
	}
	if col.builder != nil || len(col.tasks)+len(col.children) > 0 {
		o.recordMisuse(ErrInvalidConfig)
		return g
	}
	for _, p := range preds {
		if pred, ok := o.predecessorOfLocked(p); ok {
			col.entry = append(col.entry, o.closeMembershipLocked(pred))
		}
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
	col := o.tasksByRef[g.id]
	switch {
	case col == nil:
		o.recordMisuse(g.rejected)
		o.mu.Unlock()
		return
	case o.ensureOpen() != nil:
		o.recordMisuse(ErrClosed)
		o.mu.Unlock()
		return
	case col.builder != nil || len(col.tasks)+len(col.children) > 0:
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	gate := &taskState{
		id:          o.nextID("gate"),
		name:        col.name,
		rec:         o.rec.NewTask(record.TaskInit{State: Pending}),
		declaration: o.nextDecl(),
		doneCh:      make(chan struct{}),
		gateFor:     col,
	}
	gate.handle = &TaskHandle{out: o, id: gate.id}
	gate.sched.preds = slices.Clone(col.entry)
	gate.sched.work = func() error { run(); return nil }
	col.builder = &containerBuilder{gate: gate, phase: builderPending}
	for c := col; c != nil; c = c.parent {
		c.tally.holds++
	}
	o.taskByRef[gate.id] = gate
	o.sched.gates = append(o.sched.gates, gate)
	o.sched.wg.Add(1)
	o.enterPhaseLocked(gate, phaseQueued)
	if o.sched.cancelled {
		o.markNotStartedLocked(gate)
		o.mu.Unlock()
		return
	}
	o.placeLocked(gate, scanAll)
	o.mu.Unlock()
	o.kick()
}

// runGate runs a claimed container builder and settles its gate. The
// builder runs under builderFrames, not runCallback, so the declarations it
// makes are not Task-callback declarations (see declaredInCallback).
func (o *Output) runGate(st *taskState) {
	panicText := runBuilder(st.sched.work)
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

// concludeGateLocked is settleLocked's tail for a gate: it records the builder's
// outcome, releases the membership holds the pending builder placed on its
// container and ancestors, and places every Task that was waiting on them.
func (o *Output) concludeGateLocked(st *taskState) {
	col, state := st.gateFor, st.rec.State()
	switch state {
	case Done:
		col.builder.phase = builderDone
	case NotStarted:
		col.builder.phase = builderNotStarted
	default:
		col.builder.phase = builderFailed
	}
	st.closeDoneLocked()
	if st.sched.awaitingStart() {
		o.abandonLocked(st)
	}
	var woken []*taskState
	for c := col; c != nil; c = c.parent {
		if state != Done {
			woken = append(woken, c.tally.failBuilder()...)
		}
		woken = append(woken, c.tally.release(c == col, o.declSeq)...)
	}
	o.bumpLocked()
	o.wakeLocked(woken)
}

// awaitBuilders parks until every topology builder under the container has
// settled, and returns why a builder did not declare its children. Nested
// builders are declared by their parent's, so each level is awaited after
// the one above it.
func (o *Output) awaitBuilders(id string, stack *waiterStack) error {
	o.mu.Lock()
	col := o.tasksByRef[id]
	o.mu.Unlock()
	if col == nil {
		return nil
	}
	return o.awaitBuildersIn(col, stack, &inputSeals{})
}

func (o *Output) awaitBuildersIn(col *tasksState, stack *waiterStack, seen *inputSeals) error {
	o.mu.Lock()
	b := col.builder
	o.mu.Unlock()
	var errs []error
	if b != nil {
		if err := b.gate.handle.waitChecked(stack, seen); err != nil {
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
