package evo

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
type EntityState string

const (
	Pending    EntityState = "pending"
	Running    EntityState = "running"
	Done       EntityState = "done"
	Warning    EntityState = "warning"
	Blocked    EntityState = "blocked"
	Failed     EntityState = "failed"
	Skipped    EntityState = "skipped"
	Cancelled  EntityState = "cancelled"
	Empty      EntityState = "empty"
	Incomplete EntityState = "incomplete"
	// NotStarted marks a group task that never ran because an earlier sibling
	// already failed or was cancelled — rendered "-  <name>  not started" and
	// excluded from the conclusion (the group's verdict comes from the
	// failed/cancelled sibling, not from its unstarted followers).
	NotStarted EntityState = "not_started"
)

// ConclusionState is the human headline for a finished output.
type ConclusionState string

const (
	StateReady     ConclusionState = "ready"
	StateChanged   ConclusionState = "changed"
	StateUnchanged ConclusionState = "unchanged"
	StateWarning   ConclusionState = "warning"
	StateBlocked   ConclusionState = "blocked"
	StateFailed    ConclusionState = "failed"
	StateCancelled ConclusionState = "cancelled"
	StatePlanned   ConclusionState = "planned"
)

// ProgressKind classifies task measurement.
type ProgressKind string

const (
	Indeterminate ProgressKind = "indeterminate"
	Determinate   ProgressKind = "determinate"
	BytesKind     ProgressKind = "bytes"
)

// Progress is absolute measurement for a task.
type Progress struct {
	Kind      ProgressKind
	Completed int64
	Total     int64
}

// isTerminalTask reports whether s is a terminal task state.
func isTerminalTask(s EntityState) bool {
	switch s {
	case Done, Warning, Blocked, Failed, Cancelled, Skipped, NotStarted:
		return true
	default:
		return false
	}
}
