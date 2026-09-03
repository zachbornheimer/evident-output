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
// declaration order. name is a printf format when args are present
// (fmt.Sprintf semantics); the get-or-create key is the formatted name.
// args may also carry evo.ID to set a stable machine key.
func (g *GroupHandle) Task(name string, args ...any) *TaskHandle {
	formatted, opts := formatEntityName(name, args)
	return g.tasks.out.groupTaskGetOrCreate(g.tasks.id, formatted, opts...)
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
