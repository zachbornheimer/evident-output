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

// Wait blocks until every child this Group declared (directly or through a
// nested Group/Sequence) has settled, and returns their aggregate outcome —
// see internal/engine.GroupHandle.Wait (ZYS-849 Decisions).
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

// Wait blocks until every step this Sequence declared (directly or through a
// nested Group/Sequence) has settled, and returns their aggregate outcome —
// see internal/engine.SequenceHandle.Wait (ZYS-849 Decisions).
func (s *SequenceHandle) Wait() error {
	if s == nil {
		return nil
	}
	return s.impl().Wait()
}
