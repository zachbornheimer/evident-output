package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FingerprintBasis.
type BasisSource = freshness.BasisSource

// FingerprintBasis observes a Fingerprint (FSPath, Value, App).
func FingerprintBasis(f freshness.Fingerprint) BasisSource { return freshness.FingerprintBasis(f) }

// basisIsCurrent judges this Task's Basis as it starts (see
// freshness.TaskTable.JudgeBasis) and narrates the verdict. A Task with no
// Basis is never current.
func (t *TaskHandle) basisIsCurrent(ctx context.Context) (bool, error) {
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	o.mu.Unlock()
	if st == nil {
		return false, nil
	}
	judgement, err := o.taskFreshness.JudgeBasis(ctx, o.manifest, st)
	if err != nil || !judgement.Declared {
		return false, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventBasisFingerprinted, t.id, map[string]any{
		"count": len(judgement.Observed), "current": judgement.Current,
	})
	return judgement.Current, nil
}
