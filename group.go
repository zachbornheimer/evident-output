package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

type SequenceHandle struct{ inner *engine.SequenceHandle }
type GroupHandle struct{ inner *engine.GroupHandle }

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
	unwrapped := make([]any, len(preds))
	for i, p := range preds {
		unwrapped[i] = unwrapPred(p)
	}
	t.impl().After(unwrapped...)
	return t
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
