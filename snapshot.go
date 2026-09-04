package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// Snapshot is an immutable complete presentation state at a version.
//
// Declared as a type alias into internal/core (the repo's data-model
// package — see EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md §38): rendering
// and evidence-capture machinery import core, never this root package, so
// the data model has to live where they can reach it without an import
// cycle back through the behavioral facades (Output, TaskHandle, Evidence)
// that stay declared here. pkg.go.dev cannot expand an aliased type's
// fields (internal/core is never rendered) — see docs/reference.md for the
// full field-level reference this doc comment summarizes.
type Snapshot = core.Snapshot

// TaskSnapshot is an immutable task view.
type TaskSnapshot = core.TaskSnapshot

// TaxonomyRecord is one accumulated (reason, name) disposition entry —
// recorded by TaskHandle.Skipped or TaskHandle.Kept, never assembled by hand.
type TaxonomyRecord = core.TaxonomyRecord

// TasksSnapshot is an immutable collection view.
type TasksSnapshot = core.TasksSnapshot

// ChangesSnapshot is an immutable changes section.
type ChangesSnapshot = core.ChangesSnapshot

// PlanSnapshot is an immutable plan section.
type PlanSnapshot = core.PlanSnapshot

// EffectRecord is one semantic change or plan row.
type EffectRecord = core.EffectRecord
