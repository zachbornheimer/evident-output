package evo

// EntityState is the lifecycle state of an item or task.
type EntityState string

const (
	Pending    EntityState = "pending"
	Running    EntityState = "running"
	OK         EntityState = "ok"
	Done       EntityState = "done"
	Warning    EntityState = "warning"
	Blocked    EntityState = "blocked"
	Failed     EntityState = "failed"
	Unknown    EntityState = "unknown"
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
	StatePartial   ConclusionState = "partial"
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

// isTerminalItem reports whether s is a terminal item state.
func isTerminalItem(s EntityState) bool {
	switch s {
	case OK, Warning, Blocked, Failed, Unknown, Skipped, Cancelled:
		return true
	default:
		return false
	}
}

// isTerminalTask reports whether s is a terminal task state.
func isTerminalTask(s EntityState) bool {
	switch s {
	case Done, Warning, Failed, Cancelled, Skipped, NotStarted:
		return true
	default:
		return false
	}
}
