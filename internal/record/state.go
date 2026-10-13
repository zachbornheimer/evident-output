package record

// State is where a Task or collection stands in its life: not yet started,
// working, settled with one of the six Outcomes, or abandoned by the run
// without one. A settled State is spelled exactly as its Outcome, so a
// projection that shows either shows the one vocabulary CONTRACT §4 names.
type State string

const (
	Pending   State = "pending"
	Running   State = "running"
	Succeeded       = State(OutcomeSucceeded)
	Satisfied       = State(OutcomeSatisfied)
	Excluded        = State(OutcomeExcluded)
	Refused         = State(OutcomeRefused)
	Failed          = State(OutcomeFailed)
	Cancelled       = State(OutcomeCancelled)
	// Empty is a collection that settled with no members.
	Empty State = "empty"
	// Incomplete is a Task the run finished without ever settling: misuse.
	Incomplete State = "incomplete"
	// NotStarted is a Task that never ran because a predecessor or an earlier
	// sibling ended Failed, Refused, Excluded or Cancelled (CONTRACT §2).
	NotStarted State = "not_started"
)

// Outcome is the verdict s settled with, or the zero Outcome for a State
// that is not one of the six.
func (s State) Outcome() Outcome {
	switch s {
	case Succeeded, Satisfied, Excluded, Refused, Failed, Cancelled:
		return Outcome(s)
	}
	return ""
}

// Succeeded reports whether s settled with the work done: Succeeded, or
// Satisfied because it was already done.
func (s State) Succeeded() bool { return s == Succeeded || s == Satisfied }

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

// IsTerminalTask reports whether s is a terminal task state.
func IsTerminalTask(s State) bool {
	return s.Outcome() != "" || s == NotStarted
}
