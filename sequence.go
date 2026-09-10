package evo

import "iter"

// SequenceHandle is the front door for ordered child work: the same
// scheduler as Group with implicit predecessor edges in declaration order.
// A failed/blocked/cancelled child makes later siblings NotStarted.
type SequenceHandle struct {
	tasks *GroupHandle
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order.
func (g *SequenceHandle) Task(name string) *TaskHandle {
	if g == nil || g.tasks == nil {
		return &TaskHandle{}
	}
	return g.tasks.out.groupTaskGetOrCreate(g.tasks.id, name)
}

// Summary sets a success-oriented sequence summary.
func (g *SequenceHandle) Summary(text string) *SequenceHandle {
	if g == nil || g.tasks == nil {
		return g
	}
	g.tasks.Summary(text)
	return g
}

// Snapshot returns the sequence snapshot with derived state.
func (g *SequenceHandle) Snapshot() TasksSnapshot {
	if g == nil || g.tasks == nil {
		return TasksSnapshot{State: Empty}
	}
	return g.tasks.Snapshot()
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *SequenceHandle) Sequence(name string) *SequenceHandle {
	if g == nil || g.tasks == nil {
		return &SequenceHandle{}
	}
	return g.tasks.Sequence(name)
}

// Group declares (or returns) an independent child container nested here.
func (g *SequenceHandle) Group(name string) *GroupHandle {
	if g == nil || g.tasks == nil {
		return &GroupHandle{}
	}
	return g.tasks.Group(name)
}

// Each yields a child Task per item in declaration order. Range-end waits
// only for children that received Define or a mutation verb during the loop.
func (g *SequenceHandle) Each(items []string) iter.Seq2[string, *TaskHandle] {
	if g == nil || g.tasks == nil {
		return func(func(string, *TaskHandle) bool) {}
	}
	return g.tasks.Each(items)
}
