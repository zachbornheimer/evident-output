package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// EntityState is the lifecycle state of an item or task.
//
// C11 naming sweep: these members stay bare (Done, Failed, Blocked, ...)
// rather than gaining a State* prefix to match ConclusionState below —
// prefixing would collide outright with ConclusionState's own StateFailed/
// StateBlocked/StateCancelled/StateWarning constants (same package, same
// identifiers, different types is still a duplicate declaration in Go).
// Renaming ConclusionState's constants instead would ripple into the JSON
// wire (schema 0.3, frozen this release) and every existing golden — this
// is the "document instead" branch the census decision allows.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type EntityState = core.EntityState

// EntityState values — see the type doc comment above for the naming
// rationale (no State* prefix on this block).
const (
	Pending    = core.Pending
	Running    = core.Running
	Done       = core.Done
	Blocked    = core.Blocked
	Failed     = core.Failed
	Skipped    = core.Skipped
	Cancelled  = core.Cancelled
	Empty      = core.Empty
	Incomplete = core.Incomplete
	// NotStarted marks a group task that never ran because an earlier sibling
	// already failed or was cancelled — rendered "-  <name>  not started" and
	// excluded from the conclusion (the group's verdict comes from the
	// failed/cancelled sibling, not from its unstarted followers).
	NotStarted = core.NotStarted
)

// ConclusionState is the human headline for a finished output.
type ConclusionState = core.ConclusionState

// ConclusionState values — the trailing "[state]" band a run can end in.
const (
	StateReady     = core.StateReady
	StateChanged   = core.StateChanged
	StateWarning   = core.StateWarning
	StateBlocked   = core.StateBlocked
	StateFailed    = core.StateFailed
	StateCancelled = core.StateCancelled
	StatePlanned   = core.StatePlanned
)

// ProgressKind classifies task measurement.
type ProgressKind = core.ProgressKind

// ProgressKind values — which measurement a task's Progress reports.
const (
	Indeterminate = core.Indeterminate
	Determinate   = core.Determinate
	BytesKind     = core.BytesKind
)

// Progress is absolute measurement for a task.
type Progress = core.Progress
