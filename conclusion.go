package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// EvidencePhase is one Verify observation attempt (§30): a pre- or
// post-Define check, and whether it was ever evaluated.
type EvidencePhase = core.EvidencePhase

// TaskEvidence preserves both observation phases a Task's Verify may have
// recorded (§30).
type TaskEvidence = core.TaskEvidence
