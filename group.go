package evo

import "iter"

func (g *GroupHandle) Each(items []string) iter.Seq2[string, *TaskHandle] {
	if g == nil || g.inner == nil {
		return func(func(string, *TaskHandle) bool) {}
	}
	return wrapEach(g.inner.Each(items))
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

func (s *SequenceHandle) Each(items []string) iter.Seq2[string, *TaskHandle] {
	if s == nil || s.inner == nil {
		return func(func(string, *TaskHandle) bool) {}
	}
	return wrapEach(s.inner.Each(items))
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
