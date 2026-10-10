package record

// Outcome is how a Task ended: the closed set CONTRACT §4 names. It is the
// one vocabulary for a verdict; EntityState, Resolution and ConclusionState
// are older spellings derived from it (see internal/core) until the root
// package adopts it directly.
//
// The members carry an Outcome prefix only while EntityState still owns the
// bare names Failed and Cancelled in this package.
type Outcome string

const (
	// OutcomeSucceeded means the Task ran and succeeded.
	OutcomeSucceeded Outcome = "succeeded"
	// OutcomeSatisfied means the requested state was already true and nothing ran.
	OutcomeSatisfied Outcome = "satisfied"
	// OutcomeExcluded means policy chose not to run the Task.
	OutcomeExcluded Outcome = "excluded"
	// OutcomeRefused means a precondition stopped the Task before any mutation.
	OutcomeRefused Outcome = "refused"
	// OutcomeFailed means the Task ran and failed.
	OutcomeFailed Outcome = "failed"
	// OutcomeCancelled means the Task was interrupted.
	OutcomeCancelled Outcome = "cancelled"
)

// Phase is where a Task stands in its life. A Task has an Outcome only once
// its Phase is PhaseSettled.
type Phase string

const (
	// PhasePending means the Task has not started.
	PhasePending Phase = "pending"
	// PhaseRunning means the Task is working.
	PhaseRunning Phase = "running"
	// PhaseSettled means the Task ended, with exactly one Outcome.
	PhaseSettled Phase = "settled"
)
