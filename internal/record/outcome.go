package record

// Outcome is how a Task ended: the closed set CONTRACT §4 names and the only
// verdict type. A settled State is spelled as its Outcome (see State.Outcome).
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

// outcomeRanks orders the outcomes that decide a run on their own: Failed
// outranks Refused outranks Cancelled.
var outcomeRanks = map[Outcome]int{
	OutcomeFailed:    3,
	OutcomeRefused:   2,
	OutcomeCancelled: 1,
}

// RunOutcome is the Conclusion's Outcome for a run whose Tasks ended as
// outcomes (CONTRACT §4): the worst outcome decides, Failed > Refused >
// Cancelled. Excluded folds into the tally and decides only when everything
// was Excluded. Otherwise the run is Satisfied when all its work was already
// current, and Succeeded when any of it ran or there was none.
func RunOutcome(outcomes ...Outcome) Outcome {
	var worst Outcome
	excluded, satisfied, other := 0, 0, 0
	for _, outcome := range outcomes {
		if outcomeRanks[outcome] > outcomeRanks[worst] {
			worst = outcome
		}
		switch outcome {
		case OutcomeExcluded:
			excluded++
		case OutcomeSatisfied:
			satisfied++
		case "":
		default:
			other++
		}
	}
	switch {
	case worst != "":
		return worst
	case other > 0:
		return OutcomeSucceeded
	case satisfied > 0:
		return OutcomeSatisfied
	case excluded > 0:
		return OutcomeExcluded
	}
	return OutcomeSucceeded
}

// ExitCode is the process exit code CONTRACT §1 assigns a run that concluded
// outcome: 0 for Succeeded, Satisfied and Excluded, 1 for Refused, 2 for
// Failed and 130 for Cancelled.
func (o Outcome) ExitCode() int {
	switch o {
	case OutcomeRefused:
		return ExitRefused
	case OutcomeFailed:
		return ExitFailed
	case OutcomeCancelled:
		return ExitCancelled
	}
	return ExitOK
}
