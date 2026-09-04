package evo

// SequenceHandle is the front door for an ordered dependency of steps that
// must stop implying "still might run" once a child reaches Failed or Cancelled:
// every later-declared sibling still unresolved auto-resolves to NotStarted
// ("-  <name>  not started") — no caller code required. It is a thin
// identity layer over DisplayGroup (the underlying collection) that adds
// get-or-create children. Construct one via evo.Sequence or Output.Sequence.
type SequenceHandle struct {
	tasks *DisplayGroup
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order. name is a printf format when args are present
// (fmt.Sprintf semantics); the get-or-create key is the formatted name.
// args may also carry evo.ID to set a stable machine key.
func (g *SequenceHandle) Task(name string, args ...any) *TaskHandle {
	formatted, opts := formatEntityName(name, args)
	return g.tasks.out.groupTaskGetOrCreate(g.tasks.id, formatted, opts...)
}

// Summary sets a success-oriented sequence summary. text is a printf format
// when args are present (fmt.Sprintf semantics) — see Task (C6).
func (g *SequenceHandle) Summary(text string, args ...any) *SequenceHandle {
	g.tasks.Summary(text, args...)
	return g
}

// Snapshot returns the sequence snapshot with derived state.
func (g *SequenceHandle) Snapshot() TasksSnapshot {
	return g.tasks.Snapshot()
}

// Sequence declares (or, for a repeated name, returns) an ordered child
// container nested under this Sequence (P3's recursive nesting).
func (g *SequenceHandle) Sequence(name string, args ...any) *SequenceHandle {
	return g.tasks.Sequence(name, args...)
}

// DisplayGroup declares a fresh child container nested under this Sequence
// (P3's recursive nesting).
func (g *SequenceHandle) DisplayGroup(name string, args ...any) *DisplayGroup {
	return g.tasks.DisplayGroup(name, args...)
}
