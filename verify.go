package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// EvidencePhase is one Verify observation attempt (§30): a pre- or
// post-Define check, and whether it was ever evaluated.
type EvidencePhase = core.EvidencePhase

// TaskEvidence preserves both observation phases a Task's Verify may have
// recorded (§30).
type TaskEvidence = core.TaskEvidence

// Verify registers an advanced read-only check that the Task's desired
// state already holds, ANDed with any earlier check. Call it before Define.
// Define runs every check before the callback (all true resolves the Task
// AlreadySatisfied without running it) and again after a successful
// callback (any false fails the Task with ProblemCodeVerificationUnsatisfied).
// The after-check is skipped in two cases only: Define resolved the Task
// itself (Block, or Skipped with no Effect committed first), or a dry
// run or preview skipped an Effect Define planned. A planned run whose
// Define planned nothing is checked like a real one.
func (t *TaskHandle) Verify(fn func(context.Context) (bool, error)) *TaskHandle {
	t.impl().Verify(fn)
	return t
}
