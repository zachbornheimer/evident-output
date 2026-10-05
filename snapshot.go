package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Snapshot is an immutable complete presentation state at a version.
//
// Declared as a type alias into internal/core (the repo's data-model
// package — see EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md §38): rendering
// and evidence-capture machinery import core, never this root package, so
// the data model has to live where they can reach it without an import
// cycle back through the behavioral facades (Output, TaskHandle, evidence)
// that stay declared here. pkg.go.dev cannot expand an aliased type's
// fields (internal/core is never rendered) — see docs/reference.md for the
// full field-level reference this doc comment summarizes.
type Snapshot = core.Snapshot

// TaskSnapshot is an immutable task view.
type TaskSnapshot = core.TaskSnapshot

// TasksSnapshot is an immutable collection view.
type TasksSnapshot = core.TasksSnapshot

// ChangesSnapshot is an immutable changes section.
type ChangesSnapshot = core.ChangesSnapshot

// PlanSnapshot is an immutable plan section.
type PlanSnapshot = core.PlanSnapshot

// MessageSnapshot is one logical user-facing message in the canonical model.
type MessageSnapshot = core.MessageSnapshot

// Event is an immutable journal record.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type Event = core.Event

type PlainOptions = engine.PlainOptions

func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return engine.RenderPlain(s, opts)
}

func (o *Output) Snapshot() Snapshot {
	if o == nil || o.inner == nil {
		return Snapshot{}
	}
	return o.inner.Snapshot()
}

func (t *TaskHandle) Snapshot() TaskSnapshot {
	if t == nil || t.inner == nil {
		return TaskSnapshot{}
	}
	return t.inner.Snapshot()
}
