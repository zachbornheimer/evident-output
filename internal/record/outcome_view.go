package record

// EntityStateOf is the legacy task state of a Task in phase that settled to
// outcome. Pending and Running ignore outcome. It is the zero state for a
// settled phase without one of the six outcomes. Incomplete and NotStarted
// have no Outcome: they describe a Task the run abandoned, not one that
// ended.
func EntityStateOf(phase Phase, outcome Outcome) EntityState {
	switch phase {
	case PhasePending:
		return Pending
	case PhaseRunning:
		return Running
	case PhaseSettled:
		return settledEntityState(outcome)
	}
	return ""
}

func settledEntityState(outcome Outcome) EntityState {
	switch outcome {
	case OutcomeSucceeded, OutcomeSatisfied:
		return Done
	case OutcomeExcluded:
		return Skipped
	case OutcomeRefused:
		return Blocked
	case OutcomeFailed:
		return Failed
	case OutcomeCancelled:
		return Cancelled
	}
	return ""
}

// ResolutionOf is the legacy reason a Task settled successfully: Satisfied
// is AlreadySatisfied, a Succeeded whose Define callback ran is Executed
// whether or not it changed anything, and a Succeeded that never ran Define
// is NoWork. Every other outcome keeps the default, NoWork.
func ResolutionOf(outcome Outcome, defineRan bool) Resolution {
	switch {
	case outcome == OutcomeSatisfied:
		return ResolutionAlreadySatisfied
	case outcome == OutcomeSucceeded && defineRan:
		return ResolutionExecuted
	}
	return ResolutionNoWork
}

// ConclusionInputs is everything ConclusionStateOf reads about a run.
type ConclusionInputs struct {
	// Worst is the run's worst Task outcome (WorstOutcome).
	Worst Outcome
	// Changed reports a committed mutation.
	Changed bool
	// Planned reports a non-empty [planned] ledger.
	Planned bool
	// DryRun reports the planned tense.
	DryRun bool
	// AnySettledOK reports that some Task or collection ended Done or Skipped.
	AnySettledOK bool
	// Warned reports a warning-severity Problem on some Task or collection.
	Warned bool
}

// ConclusionStateOf is the legacy headline of a run described by in. Failed,
// Refused and Cancelled outrank everything and ignore DryRun; otherwise
// Changed beats Planned beats a settled OK beats Warned beats ready. A dry
// run never reads as done, but a warning-only headline stays a warning.
func ConclusionStateOf(in ConclusionInputs) ConclusionState {
	switch in.Worst {
	case OutcomeFailed:
		return StateFailed
	case OutcomeRefused:
		return StateBlocked
	case OutcomeCancelled:
		return StateCancelled
	}
	switch {
	case in.Changed && in.DryRun, in.Planned && !in.Changed:
		return StatePlanned
	case in.Changed:
		return StateChanged
	case !in.AnySettledOK && in.Warned:
		return StateWarning
	case in.DryRun:
		return StatePlanned
	}
	return StateReady
}
