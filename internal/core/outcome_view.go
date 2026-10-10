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

// ConclusionStateOf is the legacy headline of a run whose worst Task outcome
// is worst. changed reports a committed mutation, planned a non-empty
// [planned] ledger, and dryRun the planned tense. Failed, Refused and
// Cancelled outrank everything and ignore dryRun; otherwise changed beats
// planned beats ready, and a dry run never reads as done. StateWarning is
// not derivable here: it needs to know no Task ended Done.
func ConclusionStateOf(worst record.Outcome, changed, planned, dryRun bool) ConclusionState {
	switch worst {
	case record.OutcomeFailed:
		return StateFailed
	case record.OutcomeRefused:
		return StateBlocked
	case record.OutcomeCancelled:
		return StateCancelled
	}
	switch {
	case changed && dryRun, planned && !changed:
		return StatePlanned
	case changed:
		return StateChanged
	case dryRun:
		return StatePlanned
	}
	return StateReady
}
