package core

import "github.com/zachbornheimer/evident-output/internal/record"

// EntityStateOf is the legacy task state of a Task in phase that settled to
// outcome. Pending and Running ignore outcome. It is the zero state for a
// settled phase without one of the six outcomes. Incomplete and NotStarted
// have no Outcome: they describe a Task the run abandoned, not one that
// ended.
func EntityStateOf(phase record.Phase, outcome record.Outcome) EntityState {
	switch phase {
	case record.PhasePending:
		return Pending
	case record.PhaseRunning:
		return Running
	case record.PhaseSettled:
		return settledEntityState(outcome)
	}
	return ""
}

func settledEntityState(outcome record.Outcome) EntityState {
	switch outcome {
	case record.OutcomeSucceeded, record.OutcomeSatisfied:
		return Done
	case record.OutcomeExcluded:
		return Skipped
	case record.OutcomeRefused:
		return Blocked
	case record.OutcomeFailed:
		return Failed
	case record.OutcomeCancelled:
		return Cancelled
	}
	return ""
}

// ResolutionOf is the legacy reason a Task settled successfully: Satisfied
// is AlreadySatisfied, a Succeeded whose Define callback ran is Executed
// whether or not it changed anything, and a Succeeded that never ran Define
// is NoWork. Every other outcome keeps the default, NoWork.
func ResolutionOf(outcome record.Outcome, defineRan bool) Resolution {
	switch {
	case outcome == record.OutcomeSatisfied:
		return ResolutionAlreadySatisfied
	case outcome == record.OutcomeSucceeded && defineRan:
		return ResolutionExecuted
	}
	return ResolutionNoWork
}

// ConclusionInputs is everything ConclusionStateOf reads about a run.
type ConclusionInputs struct {
	// Worst is the run's worst Task outcome (record.WorstOutcome).
	Worst record.Outcome
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
	case record.OutcomeFailed:
		return StateFailed
	case record.OutcomeRefused:
		return StateBlocked
	case record.OutcomeCancelled:
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
