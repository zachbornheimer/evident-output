package record

import "time"

// Conclusion is the multidimensional meaning of a finished command.
type Conclusion struct {
	// Outcome is the run's verdict: RunOutcome over every Task's Outcome.
	Outcome Outcome
	Subject string
	// Summary is Snapshot.Summary: the evo.run "summary" and the run.finished payload's.
	Summary string
	Changed bool
	Partial bool
	// Warned reports a warning-severity Problem on some Task or collection,
	// or on the run itself: a modifier on the Outcome, never a verdict.
	Warned      bool
	Cancelled   bool
	Explanation string
	Tasks       []TaskSnapshot
	Collections []TasksSnapshot
	Changes     []ChangesSnapshot
	Plans       []PlanSnapshot
	Actions     []Action
	ExitCode    int
	// RunID identifies the Output instance that produced this Conclusion —
	// the wire v2 envelope's "run_id" (increment 4, schema/run.v3.json). Mirrors the
	// engine's existing per-instance id (Output.outputID); never generated
	// twice for one run.
	RunID string
	// StartedAt/FinishedAt are the run's wall-clock bounds, read through the
	// engine's Clock facade — the wire v2 envelope's "started_at"/
	// "finished_at"/derived "duration_ms" (schema/run.v3.json). Zero until Finish sets
	// them.
	StartedAt  time.Time
	FinishedAt time.Time
	// DryRun mirrors Snapshot.DryRun — the wire v2 envelope's "mode"
	// (apply|dry_run, schema/run.v3.json) has no other source, since Result/Conclusion
	// alone is WriteJSON's whole contract.
	DryRun bool
	// Facts mirrors Snapshot.Facts (run-scoped evo.Fact annotations) — the
	// wire v2 envelope's top-level "data.facts" (schema/run.v3.json).
	Facts []Fact
}

// Exit codes CONTRACT §1 assigns each Outcome (see Outcome.ExitCode).
const (
	ExitOK        = 0
	ExitRefused   = 1
	ExitFailed    = 2
	ExitCancelled = 130
)

// AnyRefused reports whether the run concluded Refused, or any root Task did.
func (c Conclusion) AnyRefused() bool {
	if c.Outcome == OutcomeRefused {
		return true
	}
	for _, t := range c.Tasks {
		if t.State == Refused {
			return true
		}
	}
	return false
}

// FoldLeftoverMisuse fails an otherwise exit-0 conclusion when Finish also
// recorded misuse (a never-resolved Task, a double resolve, ...), before the
// conclusion is rendered, so the band that prints and the exit code are the
// same fact. A Refused, Failed or Cancelled conclusion keeps its Outcome:
// misuse never overrides an ending the run reached for a reason of its own.
func FoldLeftoverMisuse(c *Conclusion, misuse error) {
	if c == nil || misuse == nil || c.ExitCode != ExitOK {
		return
	}
	c.Outcome = OutcomeFailed
	c.ExitCode = ExitFailed
	c.Explanation = misuse.Error()
}

// anyTaskWarned reports whether any task in tasks carries at least one
// warning-severity Problem.
func anyTaskWarned(tasks []TaskSnapshot) bool {
	for _, t := range tasks {
		if len(t.Warnings) > 0 {
			return true
		}
	}
	return false
}

// anyCollectionWarned recurses into every container's own tasks and nested
// containers, so a warned child at any depth marks the run Warned.
func anyCollectionWarned(collections []TasksSnapshot) bool {
	for _, col := range collections {
		if anyTaskWarned(col.Tasks) || anyCollectionWarned(col.Collections) {
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
		Summary:     s.Summary,
		Tasks:       s.Tasks,
		Collections: s.Collections,
		Changes:     s.Changes,
		Plans:       s.Plans,
		Actions:     s.Actions,
		DryRun:      s.DryRun,
		Facts:       s.Facts,
	}
	// A Changes section with zero records is a bare declaration that never
	// recorded a mutation: it does not make the run Changed.
	for _, ch := range s.Changes {
		if len(ch.Records) > 0 {
			c.Changed = true
			break
		}
	}
	states := settledStates(nil, s.Tasks, s.Collections)
	outcomes := make([]Outcome, 0, len(states))
	for _, state := range states {
		outcomes = append(outcomes, state.Outcome())
		c.Partial = c.Partial || unsettled(state)
		c.Cancelled = c.Cancelled || state == Cancelled
	}
	c.Outcome = RunOutcome(outcomes...)
	if c.Outcome.ExitCode() != ExitOK {
		c.Explanation = decidingEnding(c.Outcome, s.Tasks, s.Collections)
	}
	c.Warned = anyTaskWarned(s.Tasks) || anyCollectionWarned(s.Collections) || len(s.Warnings) > 0
	c.ExitCode = c.Outcome.ExitCode()
	return c
}

// unsettled reports whether s is a Task or collection the run left without
// an Outcome: Partial marks the Outcome incomplete instead of replacing it.
func unsettled(s State) bool {
	switch s {
	case Pending, Running, NotStarted:
		return true
	}
	return false
}

// settledStates appends the State of every Task at any depth, and of every
// collection that ended badly on its own account (a failed builder, say),
// so the Outcome is decided by the Tasks themselves: a collection whose
// members all settled well adds nothing its members did not already say.
func settledStates(into []State, tasks []TaskSnapshot, collections []TasksSnapshot) []State {
	for _, t := range tasks {
		into = append(into, t.State)
	}
	for _, col := range collections {
		if col.State.Outcome() == "" || !col.State.Succeeded() && col.State != Excluded {
			into = append(into, col.State)
		}
		into = settledStates(into, col.Tasks, col.Collections)
	}
	return into
}

// decidingEnding is the ending of the Task, at any depth, that settled first
// with outcome: why its Outcome became the run's. When several Tasks end the
// run's way, the earliest settle decides, whatever the declaration order. It
// is the Task's Summary, which its ending writes, else the Problem its ending
// recorded last; a non-fatal Problem recorded before the ending never
// explains the run.
func decidingEnding(outcome Outcome, tasks []TaskSnapshot, collections []TasksSnapshot) string {
	var earliest *TaskSnapshot
	visitTasks(tasks, collections, func(t *TaskSnapshot) {
		if t.State.Outcome() == outcome && (earliest == nil || t.SettleOrder < earliest.SettleOrder) {
			earliest = t
		}
	})
	switch {
	case earliest == nil:
		return ""
	case earliest.Summary != "" || len(earliest.Problems) == 0:
		return earliest.Summary
	}
	return earliest.Problems[len(earliest.Problems)-1].Summary
}

// visitTasks calls visit with every Task in tasks and, recursively, in collections.
func visitTasks(tasks []TaskSnapshot, collections []TasksSnapshot, visit func(*TaskSnapshot)) {
	for i := range tasks {
		visit(&tasks[i])
	}
	for _, col := range collections {
		visitTasks(col.Tasks, col.Collections, visit)
	}
}
