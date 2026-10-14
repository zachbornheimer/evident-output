package record

import "time"

// TaskTiming is when a Task was declared, began running and settled, read
// from the engine's clock. A zero time means the moment never happened.
type TaskTiming struct {
	Declared time.Time
	Started  time.Time
	Settled  time.Time
}

// TaskOperation is one tracked File or Exec operation a Task performed or
// carried forward from the manifest.
type TaskOperation struct {
	Kind string
	// Executed is false for an operation the manifest proved current and the
	// Task therefore did not run again.
	Executed bool
	// Current is true when the operation's record holds this run's state.
	// An operation that ran and failed left no such record.
	Current bool
}

// TaskBasisEntry is one input identity a Task's operations observed.
type TaskBasisEntry struct {
	Kind string
	Key  string
}

// TaskResource is one resource a Task's operations track, with the identity
// it had when the operation recorded it.
type TaskResource struct {
	Kind        string
	Path        string
	Fingerprint string
}

// TaskOperations is what a Task's tracked operations observed and produced.
type TaskOperations struct {
	Operations []TaskOperation
	Basis      []TaskBasisEntry
	Resources  []TaskResource
}
