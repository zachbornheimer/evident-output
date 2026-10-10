package engine

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// fileCallFor is the part of one File call that defines it for freshness.
func fileCallFor(path string, spec FileSpec, basis []freshness.BasisRecord) freshness.FileCall {
	return freshness.FileCall{
		Path:            path,
		ContentsManaged: spec.Contents != nil,
		Contents:        spec.Contents,
		Mode:            uint32(spec.Mode),
		Basis:           basis,
	}
}

// fileObserveBasis opens this Run's manifest — acquiring its cross-process
// lock before File claims any resource — and observes spec.Basis under
// read claims (see observeBasis).
func (o *Output) fileObserveBasis(ctx context.Context, taskID string, spec FileSpec, path string) ([]freshness.BasisRecord, error) {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return nil, fmt.Errorf("evo: File %q: %w", path, openErr)
	}
	o.emitManifestWarningOnce(store.Warning())

	basis, observeErr := o.observeBasis(ctx, spec.Basis)
	if observeErr != nil {
		return nil, fmt.Errorf("evo: File %q: %w", path, observeErr)
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventBasisFingerprinted, taskID, map[string]any{
		"path": path, "count": len(basis),
	})
	o.mu.Unlock()
	return basis, nil
}

// fileConsultManifest resolves this File call's prior operation record (if
// any) and reports whether it is still current against the observed basis
// (spec §11.4/§11.5), plus the freshness reason (spec §38: "Basis drift"
// vs "tracked output drift" vs no prior record must be distinguishable).
// prior is always returned so the caller can carry it forward unchanged on
// a current hit.
func (o *Output) fileConsultManifest(ctx context.Context, taskID string, spec FileSpec, path string, basis []freshness.BasisRecord) (current bool, prior freshness.OperationRecord, reason string, err error) {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return false, freshness.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", path, openErr)
	}
	o.mu.Lock()
	key, ord, ok := o.taskManifestKeyLocked(taskID)
	o.mu.Unlock()
	if !ok {
		return false, freshness.OperationRecord{}, "", ErrNoTaskContext
	}
	verdict, consultErr := fileCallFor(path, spec, basis).Consult(ctx, store, key, ord)
	if consultErr != nil {
		return false, freshness.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", path, consultErr)
	}
	return verdict.Current, verdict.Prior, verdict.Reason, nil
}

// fileRecordOperation persists op's freshly committed state as its Task's
// next pending manifest record, committed only once the Task itself
// settles Done (spec §8.2/§11.3).
func (o *Output) fileRecordOperation(ctx context.Context, op fileOperation) error {
	rec, recordErr := fileCallFor(op.path, op.spec, op.basis).Record(ctx)
	if recordErr != nil {
		return fmt.Errorf("evo: File %q: %w", op.path, recordErr)
	}
	o.mu.Lock()
	o.appendManifestOperationLocked(op.taskID, rec)
	o.mu.Unlock()
	return nil
}
