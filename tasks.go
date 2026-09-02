package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Tasks is a handle for a collection of independent child tasks.
// State is always derived from children; no Done/Fail/Progress methods.
type Tasks struct {
	out *Output
	id  string
}

// Task declares a child task in declaration order. Optional evo.ID sets a stable machine key.
func (g *Tasks) Task(name string, opts ...EntityOption) *TaskHandle {
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return &TaskHandle{out: g.out, id: g.out.nextID("task")}
	}
	eo := applyEntityOptions(opts)
	return g.out.addTaskLocked(sanitize.Text(name), col, eo.key)
}

// Summary sets a success-oriented collection summary.
func (g *Tasks) Summary(text string) *Tasks {
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return g
	}
	if err := g.out.ensureOpen(); err != nil {
		g.out.recordMisuse(err)
		return g
	}
	col.summary = sanitize.Text(text)
	g.out.bumpLocked()
	g.out.appendEventLocked(Event{Type: "tasks.summary_set", EntityID: g.id})
	return g
}

// Summaryf sets a formatted success-oriented collection summary.
func (g *Tasks) Summaryf(format string, args ...any) *Tasks {
	return g.Summary(fmt.Sprintf(format, args...))
}

// Snapshot returns the collection snapshot with derived state.
func (g *Tasks) Snapshot() TasksSnapshot {
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return TasksSnapshot{ID: g.id, State: Empty}
	}
	return col.snapshot()
}
