package core

// Resolution names why a Task settled successfully — a successful
// resolution reason (§29), never a lifecycle state of its own.
type Resolution string

const (
	// ResolutionExecuted marks a Task whose Define callback was entered and
	// returned successfully.
	ResolutionExecuted Resolution = "executed"
	// ResolutionAlreadySatisfied marks a Task whose Define callback was
	// never entered because a pre-Define Verify already proved the desired
	// state.
	ResolutionAlreadySatisfied Resolution = "already_satisfied"
	// ResolutionNoWork marks a Task explicitly resolved successfully
	// without ever reaching Define — the pre-Define-model default.
	ResolutionNoWork Resolution = "no_work"
)

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
