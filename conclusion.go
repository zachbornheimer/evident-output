package evo

// Conclusion is the multidimensional meaning of a finished command.
type Conclusion struct {
	State       ConclusionState
	Subject     string
	Changed     bool
	Partial     bool
	Cancelled   bool
	Explanation string
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

// allChildrenUnchanged reports whether every child task is a Done
// resolution tagged Task.Unchanged (I7) — an empty child list never counts
// (a collection with no children says nothing about "unchanged").
func allChildrenUnchanged(children []TaskSnapshot) bool {
	if len(children) == 0 {
		return false
	}
	for _, t := range children {
		if t.State != Done || !t.unchanged {
			return false
		}
	}
	return true
}

func inferConclusion(s Snapshot) Conclusion {
	c := Conclusion{
		Subject:     s.Subject,
		Tasks:       s.Tasks,
		Collections: s.Collections,
		Changes:     s.Changes,
		Plans:       s.Plans,
		Actions:     s.Actions,
	}
	// A Changes section with zero records is a bare declaration that never
	// recorded a mutation — it must not make the run read as Changed
	// (evo-rec.md "Empty effect section grammar + Changed flag").
	for _, ch := range s.Changes {
		if len(ch.Records) > 0 {
			c.Changed = true
			break
		}
	}

	var (
		hasFailed     bool
		hasBlocked    bool
		hasWarning    bool
		hasCancelled  bool
		hasIncomplete bool
		hasDone       bool
		// doneCount/unchangedCount let a run made entirely of Task.Unchanged
		// resolutions conclude StateUnchanged instead of the generic
		// StateReady an ordinary Done gets (I7) — Skipped doesn't carry the
		// tag and never counts as "unchanged" on its own.
		doneCount      int
		unchangedCount int
	)

	for _, t := range s.Tasks {
		switch t.State {
		case Failed:
			hasFailed = true
		case Blocked:
			hasBlocked = true
		case Warning:
			hasWarning = true
		case Cancelled:
			hasCancelled = true
		case Pending, Running, Incomplete, NotStarted:
			hasIncomplete = true
		case Done, Skipped:
			hasDone = true
			doneCount++
			if t.State == Done && t.unchanged {
				unchangedCount++
			}
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
			doneCount++
			// A collection counts toward "all unchanged" only when every one
			// of its children does (collections derive their tally from
			// children, never their own bare marker).
			if col.State == Done && allChildrenUnchanged(col.Tasks) {
				unchangedCount++
			}
		}
	}

	// Headline precedence: failed > blocked > cancelled > changed > planned >
	// partial > ready > warning-only > unchanged.
	//
	// Warnings sit below every OK-family outcome (evo-rec.md "Conclusion
	// algebra — two axes"): Outcome is OK|Blocked|Failed|Cancelled, and a
	// warning is not one of those four — it is visible attention on its own
	// "!" row, never a headline that overrides an otherwise-OK verdict. It
	// only becomes the headline when nothing else in the run classifies
	// (the run's only content is a warning, per TestDOM038_WarningOnly).
	switch {
	case hasFailed:
		c.State = StateFailed
	case hasBlocked:
		c.State = StateBlocked
	case hasCancelled:
		c.State = StateCancelled
		c.Cancelled = true
	case c.Changed:
		c.State = StateChanged
	case len(s.Plans) > 0 && !c.Changed && !hasFailed && !hasBlocked:
		c.State = StatePlanned
	case hasDone && doneCount > 0 && doneCount == unchangedCount:
		c.State = StateUnchanged
	case hasDone:
		c.State = StateReady
	case hasWarning:
		c.State = StateWarning
	default:
		c.State = StateUnchanged
	}

	// Partial is a completeness modifier over the Outcome above, never a root
	// verdict of its own (evo-rec.md "Conclusion algebra — two axes"): an
	// unresolved task/collection never invents a new headline state —
	// it marks the existing outcome incomplete instead.
	if hasIncomplete {
		c.Partial = true
	}
	if hasCancelled {
		c.Cancelled = true
	}

	// DryRun never lets the headline read as done: a run that would
	// otherwise conclude OK (ready/changed/unchanged) reads as planned
	// instead — nothing was actually applied (evo-rec.md Problem 1).
	if s.DryRun {
		switch c.State {
		case StateChanged, StateReady, StateUnchanged:
			c.State = StatePlanned
		}
	}

	c.ExitCode = exitCodeFor(c.State)
	return c
}
