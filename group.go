package evo

// GroupHandle is the front door for a sequence of steps that must stop on
// failure: once a child reaches Failed or Cancelled, every later-declared
// sibling still unresolved auto-resolves to NotStarted
// ("-  <name>  not started") — no caller code required. It is a thin
// identity layer over Tasks (the existing Output.Tasks collection) that
// adds get-or-create children. Construct one via evo.Group or Output.Group.
type GroupHandle struct {
	tasks *Tasks
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order. Optional evo.ID sets a stable machine key.
func (g *GroupHandle) Task(name string, opts ...EntityOption) *TaskHandle {
	return g.tasks.out.groupTaskGetOrCreate(g.tasks.id, name, opts...)
}

// Summary sets a success-oriented group summary.
func (g *GroupHandle) Summary(text string) *GroupHandle {
	g.tasks.Summary(text)
	return g
}

// Summaryf sets a formatted success-oriented group summary.
func (g *GroupHandle) Summaryf(format string, args ...any) *GroupHandle {
	g.tasks.Summaryf(format, args...)
	return g
}

// Snapshot returns the group snapshot with derived state.
func (g *GroupHandle) Snapshot() TasksSnapshot {
	return g.tasks.Snapshot()
}
