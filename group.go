package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

type (
	SequenceHandle struct{ inner *engine.SequenceHandle }
	GroupHandle    struct{ inner *engine.GroupHandle }
)

// Container is a node that can declare work beneath it: the run, a Group,
// or a Sequence. A Task is never a Container.
//
// Shared topology builders take a Container:
//
//	func Build(parent evo.Container)
type Container interface {
	Task(name string) *TaskHandle
	Group(name string) *GroupHandle
	Sequence(name string) *SequenceHandle
}

var (
	_ Container = (*Output)(nil)
	_ Container = (*GroupHandle)(nil)
	_ Container = (*SequenceHandle)(nil)
)

// Sequence declares a self-managing, ordered task container on the default
// instance.
func Sequence(name string) *SequenceHandle {
	return wrapSequence(engine.Sequence(name))
}

func Group(name string) *GroupHandle { return wrapGroup(engine.Group(name)) }

func (o *Output) Group(name string) *GroupHandle { return wrapGroup(o.impl().Group(name)) }

func (o *Output) Sequence(name string) *SequenceHandle {
	return wrapSequence(o.impl().Sequence(name))
}

func (t *TaskHandle) After(preds ...any) *TaskHandle {
	t.impl().After(unwrapPreds(preds)...)
	return t
}

func unwrapPreds(preds []any) []any {
	unwrapped := make([]any, len(preds))
	for i, p := range preds {
		unwrapped[i] = unwrapPred(p)
	}
	return unwrapped
}

// After declares predecessors: this Group declares its children only once
// every one of them succeeded, and never does once one of them cannot. A
// predecessor is a Task, a *Computed, a Group, or a Sequence, in any
// container. Call After before Define and before declaring a child.
func (g *GroupHandle) After(preds ...any) *GroupHandle {
	g.impl().After(unwrapPreds(preds)...)
	return g
}

// After is GroupHandle.After for a Sequence.
func (s *SequenceHandle) After(preds ...any) *SequenceHandle {
	s.impl().After(unwrapPreds(preds)...)
	return s
}

// Define defers this Group's children until its After predecessors have
// succeeded. build runs once, then, and declares the children through the
// Group it receives; they run with the Group's normal concurrent
// semantics, so a Skipped or Define on a child is the ordinary per-Task API.
//
// build is topology only: it takes no context, returns no error, and must
// not Wait. When a predecessor failed, build never runs and the Group
// settles NotStarted. Declaring Tasks or containers from inside a Task's
// Define callback is misuse (ErrDeclaredInCallback).
func (g *GroupHandle) Define(build func(*GroupHandle)) {
	if build == nil {
		g.impl().Define(nil)
		return
	}
	g.impl().Define(func(inner *engine.GroupHandle) { build(wrapGroup(inner)) })
}

// Define is GroupHandle.Define for a Sequence: build declares ordered steps.
func (s *SequenceHandle) Define(build func(*SequenceHandle)) {
	if build == nil {
		s.impl().Define(nil)
		return
	}
	s.impl().Define(func(inner *engine.SequenceHandle) { build(wrapSequence(inner)) })
}

func wrapSequence(inner *engine.SequenceHandle) *SequenceHandle {
	return wrap(inner, func() *SequenceHandle { return &SequenceHandle{inner: inner} })
}

func wrapGroup(inner *engine.GroupHandle) *GroupHandle {
	return wrap(inner, func() *GroupHandle { return &GroupHandle{inner: inner} })
}

func (g *GroupHandle) impl() *engine.GroupHandle {
	if g == nil {
		return nil
	}
	return g.inner
}

func (s *SequenceHandle) impl() *engine.SequenceHandle {
	if s == nil {
		return nil
	}
	return s.inner
}

func unwrapPred(p any) any {
	switch x := p.(type) {
	case *TaskHandle:
		if x == nil {
			return (*engine.TaskHandle)(nil)
		}
		return x.inner
	case *GroupHandle:
		if x == nil {
			return (*engine.GroupHandle)(nil)
		}
		return x.inner
	case *SequenceHandle:
		if x == nil {
			return (*engine.SequenceHandle)(nil)
		}
		return x.inner
	case interface{ producer() *engine.TaskHandle }:
		return x.producer()
	default:
		return p
	}
}

func (g *GroupHandle) Group(name string) *GroupHandle {
	return wrapGroup(g.impl().Group(name))
}

func (g *GroupHandle) Sequence(name string) *SequenceHandle {
	return wrapSequence(g.impl().Sequence(name))
}

func (g *GroupHandle) Snapshot() TasksSnapshot {
	if g == nil || g.inner == nil {
		return TasksSnapshot{}
	}
	return g.inner.Snapshot()
}

func (g *GroupHandle) Summary(text string) *GroupHandle {
	g.impl().Summary(text)
	return g
}

func (g *GroupHandle) Task(name string) *TaskHandle {
	return wrapTask(g.impl().Task(name))
}

// Wait blocks until every Task this Group declared, directly or through a
// nested Group/Sequence, has settled, and returns their aggregate outcome.
// It never serializes siblings: each child was already submitted by its own
// Define, and Wait only parks on outcomes the scheduler produces
// concurrently.
//
// Wait returns nil only when every descendant ran and succeeded. Otherwise
// it returns every meaningful child error, joined with errors.Join in
// declaration order, so errors.Is and errors.As reach each one. A child
// that never started because a failed sibling came first is omitted, since
// that sibling's error already explains it. When no child ran at all (for
// example, every child waited on a predecessor outside the Group that
// failed), Wait returns ErrNotStarted. A Group whose declaration was
// refused (a duplicate name, a closed Output) returns ErrNotStarted wrapping
// the refusal. Per-child detail stays in Snapshot.
//
// Calling Wait while holding a resource claim (inside an Effect, File, or
// Basis) returns ErrNestedResourceAcquisition without waiting.
func (g *GroupHandle) Wait() error {
	if g == nil {
		return nil
	}
	return g.impl().Wait()
}

func (s *SequenceHandle) Group(name string) *GroupHandle {
	return wrapGroup(s.impl().Group(name))
}

func (s *SequenceHandle) Sequence(name string) *SequenceHandle {
	return wrapSequence(s.impl().Sequence(name))
}

func (s *SequenceHandle) Snapshot() TasksSnapshot {
	if s == nil || s.inner == nil {
		return TasksSnapshot{}
	}
	return s.inner.Snapshot()
}

func (s *SequenceHandle) Summary(text string) *SequenceHandle {
	s.impl().Summary(text)
	return s
}

func (s *SequenceHandle) Task(name string) *TaskHandle {
	return wrapTask(s.impl().Task(name))
}

// Wait blocks until every step this Sequence declared, directly or through a
// nested Group/Sequence, has settled, and returns their aggregate outcome
// under the same rules as GroupHandle.Wait: nil only when every step ran
// and succeeded; meaningful errors joined in declaration order; steps that
// never started behind a failed step omitted; ErrNotStarted when no step
// ran at all; ErrNestedResourceAcquisition when called under a held
// resource claim.
func (s *SequenceHandle) Wait() error {
	if s == nil {
		return nil
	}
	return s.impl().Wait()
}
