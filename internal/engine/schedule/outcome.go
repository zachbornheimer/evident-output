// Package schedule holds the scheduler's pure decision state: predecessor
// outcomes and the descendant tallies a Group/Sequence keeps for the Tasks
// that start after it.
package schedule

import "github.com/zachbornheimer/evident-output/internal/core"

// Outcome is what a predecessor tells the Tasks after it.
type Outcome uint8

const (
	// Pending: it may still succeed.
	Pending Outcome = iota
	Succeeded
	// Failed: it can never succeed, so its dependents never start.
	Failed
)

// OutcomeOf classifies a Task state for the Tasks after it.
func OutcomeOf(s core.EntityState) Outcome {
	switch s {
	case core.Done, core.Skipped:
		return Succeeded
	case core.Failed, core.Blocked, core.Cancelled, core.NotStarted:
		return Failed
	default:
		return Pending
	}
}
