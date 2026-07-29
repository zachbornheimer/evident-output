package evo

// Conclusion is the multidimensional meaning of a finished command.
type Conclusion struct {
	State       ConclusionState
	Subject     string
	Changed     bool
	Partial     bool
	Cancelled   bool
	Explanation string
	Items       []ItemSnapshot
	Tasks       []TaskSnapshot
	Collections []TasksSnapshot
	Changes     []ChangesSnapshot
	Plans       []PlanSnapshot
	Actions     []Action
	ExitCode    int
}

// Default exit codes from architecture §26.
const (
	ExitOK        = 0
	ExitBlocked   = 1
	ExitFailed    = 2
	ExitCancelled = 130
)

func exitCodeFor(state ConclusionState) int {
	switch state {
	case StateFailed:
		return ExitFailed
	case StateBlocked:
		return ExitBlocked
	case StateCancelled:
		return ExitCancelled
	default:
		return ExitOK
	}
}

// applyFailedExitCode overrides ExitFailed when Config.FailedExitCode is set.
// code == 0 means keep the library default (ExitFailed = 2).
func applyFailedExitCode(c *Conclusion, code int) {
	if c == nil || code == 0 || c.State != StateFailed {
		return
	}
	c.ExitCode = code
}

func inferConclusion(s Snapshot) Conclusion {
	c := Conclusion{
		Subject:     s.Subject,
		Items:       s.Items,
		Tasks:       s.Tasks,
		Collections: s.Collections,
		Changes:     s.Changes,
		Plans:       s.Plans,
		Actions:     s.Actions,
	}
	if len(s.Changes) > 0 {
		c.Changed = true
	}

	var (
		hasFailed     bool
		hasBlocked    bool
		hasWarning    bool
		hasCancelled  bool
		hasIncomplete bool
		hasDone       bool
	)

	for _, it := range s.Items {
		switch it.State {
		case Failed:
			hasFailed = true
		case Blocked:
			hasBlocked = true
		case Warning:
			hasWarning = true
		case Pending, Running, Incomplete:
			hasIncomplete = true
		case OK, Skipped, Unknown:
			hasDone = true
		}
	}
	for _, t := range s.Tasks {
		switch t.State {
		case Failed:
			hasFailed = true
		case Warning:
			hasWarning = true
		case Cancelled:
			hasCancelled = true
		case Pending, Running, Incomplete:
			hasIncomplete = true
		case Done, Skipped:
			hasDone = true
		}
	}
	for _, col := range s.Collections {
		switch col.State {
		case Failed:
			hasFailed = true
		case Warning:
			hasWarning = true
		case Cancelled:
			hasCancelled = true
		case Incomplete, Running, Pending:
			hasIncomplete = true
		case Done, Empty:
			hasDone = true
		}
	}

	_ = hasDone

	// Headline precedence: failed > blocked > warning > cancelled > changed > planned > ready > unchanged
	switch {
	case hasFailed:
		c.State = StateFailed
	case hasBlocked:
		c.State = StateBlocked
	case hasWarning:
		c.State = StateWarning
	case hasCancelled:
		c.State = StateCancelled
		c.Cancelled = true
	case c.Changed:
		c.State = StateChanged
	case len(s.Plans) > 0 && !c.Changed && !hasFailed && !hasBlocked:
		c.State = StatePlanned
	case hasIncomplete:
		c.State = StatePartial
		c.Partial = true
	case s.Subject != "" || len(s.Items)+len(s.Tasks)+len(s.Collections) > 0:
		c.State = StateReady
	default:
		c.State = StateUnchanged
	}

	if hasIncomplete && (hasFailed || hasBlocked || c.Changed) {
		c.Partial = true
	}
	if hasCancelled {
		c.Cancelled = true
	}

	c.ExitCode = exitCodeFor(c.State)
	return c
}
