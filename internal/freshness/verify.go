// This file owns Verify: the current-state observation checks a Task
// registers (§9.1) and their combined verdict.

package freshness

import (
	"context"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// Verifier is one Verify observation check (§9.1): true means the desired
// state already holds, false means it does not, and an error means the
// observation itself failed.
type Verifier func(context.Context) (bool, error)

// VerifierOutcome is how one Verifier's run ended.
type VerifierOutcome int

const (
	// VerifierSatisfied: the desired state holds.
	VerifierSatisfied VerifierOutcome = iota
	// VerifierUnsatisfied: the desired state does not hold.
	VerifierUnsatisfied
	// VerifierErrored: the observation itself failed.
	VerifierErrored
)

// outcomeOf classifies one verifier's result.
func outcomeOf(ok bool, err error) VerifierOutcome {
	switch {
	case err != nil:
		return VerifierErrored
	case ok:
		return VerifierSatisfied
	default:
		return VerifierUnsatisfied
	}
}

// AddVerifier registers v as one of id's checks, ANDed with any registered
// before it.
func (t *TaskTable) AddVerifier(id record.TaskID, v Verifier) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entryLocked(id)
	entry.verifiers = append(entry.verifiers, v)
}

// Verifiers is a copy of id's registered checks, in registration order.
func (t *TaskTable) Verifiers(id record.TaskID) []Verifier {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry := t.tasks[id]; entry != nil {
		return slices.Clone(entry.verifiers)
	}
	return nil
}

// EvaluateVerifiers runs every verifier in registration order, ANDing their
// results, and stops at the first observation error (§9.1: an observation
// failure is distinct from a false result and takes priority). observed is
// told each verifier's index and outcome as it ends, so the caller can
// report it; it may be nil.
func EvaluateVerifiers(ctx context.Context, verifiers []Verifier, observed func(index int, outcome VerifierOutcome)) (allSatisfied bool, err error) {
	allSatisfied = true
	for i, v := range verifiers {
		ok, verifyErr := v(ctx)
		if observed != nil {
			observed(i, outcomeOf(ok, verifyErr))
		}
		if verifyErr != nil {
			return false, verifyErr
		}
		if !ok {
			allSatisfied = false
		}
	}
	return allSatisfied, nil
}
