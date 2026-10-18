package record

// EvidencePhase is one observation attempt (§30): a pre- or post-Define
// Verify check. Satisfied and Source are meaningful only when Evaluated is
// true — an unevaluated phase (no modeled proof was available) leaves them
// at their zero value rather than a misleading false.
type EvidencePhase struct {
	Evaluated bool
	Satisfied bool
	// Source names what produced this observation (e.g. "verify"). Empty
	// when Evaluated is false.
	Source string
}

// TaskEvidence preserves both observation phases a Task's Verify may have
// recorded, so a before-false/after-true transition is never collapsed
// into one final boolean (§30).
type TaskEvidence struct {
	Before EvidencePhase
	After  EvidencePhase
}
