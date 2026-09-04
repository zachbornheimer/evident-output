package core

// Conclusion is the multidimensional meaning of a finished command.
type Conclusion struct {
	State   ConclusionState
	Subject string
	Changed bool
	Partial bool
	// Warned reports that at least one task or collection resolved Warning
	// while the headline State settled on something else (release-gate
	// round 8 finding 3) — an otherwise-OK run must not read as silently
	// clean just because Warning sits below the OK-family headline in
	// precedence. Always false when State is itself StateWarning: that
	// headline already says it (see InferConclusion).
	Warned      bool
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

// AnyBlocked reports whether the finished conclusion is blocked, or any task snapshot is.
func (c Conclusion) AnyBlocked() bool {
	if c.State == StateBlocked {
		return true
	}
	for _, t := range c.Tasks {
		if t.State == Blocked {
			return true
		}
	}
	return false
}

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

// ApplyFailedExitCode overrides ExitFailed when Config.FailedExitCode is set.
// code == 0 means keep the library default (ExitFailed = 2).
func ApplyFailedExitCode(c *Conclusion, code int) {
	if c == nil || code == 0 || c.State != StateFailed {
		return
	}
	c.ExitCode = code
}

// FoldLeftoverMisuse demotes an otherwise-OK-family conclusion when Finish
// also recorded bookkeeping misuse (a never-resolved bare task, a
// double-resolve, ...), before the conclusion is ever rendered — so the band
// that prints and the exit code that follows it are always the same fact
// (release-gate finding 2). A conclusion that already reads Blocked/Failed/
// Cancelled for a real, documented reason keeps that verdict: misuse never
// overrides an outcome the run already printed for a reason of its own.
func FoldLeftoverMisuse(c *Conclusion, misuse error) {
	if c == nil || misuse == nil || c.ExitCode != ExitOK {
		return
	}
	c.State = StateFailed
	c.ExitCode = ExitFailed
}

// anyTaskWarned reports whether any task in tasks carries at least one
// TaskHandle.Warn annotation (P2: conclusion algebra reads TaskSnapshot.
// Warnings, never a lifecycle state — Warning is not one of the terminal
// EntityState values).
func anyTaskWarned(tasks []TaskSnapshot) bool {
	for _, t := range tasks {
		if len(t.Warnings) > 0 {
			return true
		}
	}
	return false
}

// InferConclusion derives the multidimensional meaning of a finished command
// from its final Snapshot.
func InferConclusion(s Snapshot) Conclusion {
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
		hasCancelled  bool
		hasIncomplete bool
		hasDone       bool
	)

	for _, t := range s.Tasks {
		switch t.State {
		case Failed:
			hasFailed = true
		case Blocked:
			hasBlocked = true
		case Cancelled:
			hasCancelled = true
		case Pending, Running, Incomplete, NotStarted:
			hasIncomplete = true
		case Done, Skipped:
			hasDone = true
		}
	}
	for _, col := range s.Collections {
		switch col.State {
		case Failed:
			hasFailed = true
		case Cancelled:
			hasCancelled = true
		case Incomplete, Running, Pending:
			hasIncomplete = true
		case Done, Empty:
			hasDone = true
		}
	}
	// hasWarning reads TaskSnapshot.Warnings (P2), never a lifecycle
	// EntityState — Warn annotates a task, it never resolves one.
	hasWarning := anyTaskWarned(s.Tasks)

	// Headline precedence: failed > blocked > cancelled > changed > planned >
	// ready > warning-only.
	//
	// Warnings sit below every OK-family outcome (evo-rec.md "Conclusion
	// algebra — two axes"): Outcome is OK|Blocked|Failed|Cancelled, and a
	// warning is not one of those four — it is visible attention on its own
	// "!" row, never a headline that overrides an otherwise-OK verdict. It
	// only becomes the headline when nothing else in the run classifies —
	// which, since Warn no longer resolves its task, requires an as-yet
	// unbuilt output-level Warn (P8/Facts territory); kept for that future
	// reachability and because it costs nothing to keep the algebra total.
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
	case hasDone:
		c.State = StateReady
	case hasWarning:
		c.State = StateWarning
	default:
		c.State = StateReady
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
	if hasWarning && c.State != StateWarning {
		c.Warned = true
	}

	// DryRun never lets the headline read as done: a run that would
	// otherwise conclude OK (ready/changed) reads as planned instead —
	// nothing was actually applied (evo-rec.md Problem 1).
	if s.DryRun {
		switch c.State {
		case StateChanged, StateReady:
			c.State = StatePlanned
		}
	}

	c.ExitCode = exitCodeFor(c.State)
	return c
}
