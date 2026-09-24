package evo

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
// failed), Wait returns ErrNotStarted. Per-child detail stays in Snapshot.
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
