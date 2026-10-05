package engine

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// fileObserveBasis opens this Run's manifest — acquiring its cross-process
// lock before File claims any resource — and observes spec.Basis under
// read claims (see observeBasis).
func (o *Output) fileObserveBasis(ctx context.Context, taskID string, spec FileSpec, path string) ([]manifest.BasisRecord, error) {
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
func (o *Output) fileConsultManifest(ctx context.Context, taskID string, spec FileSpec, path string, basis []manifest.BasisRecord) (current bool, prior manifest.OperationRecord, reason string, err error) {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return false, manifest.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", path, openErr)
	}
	defFingerprint := fileDefinitionFingerprint(path, spec.Contents != nil, spec.Contents, uint32(spec.Mode), basis)

	o.mu.Lock()
	key, ord, ok := o.taskManifestKeyLocked(taskID)
	o.mu.Unlock()
	if !ok {
		return false, manifest.OperationRecord{}, "", ErrNoTaskContext
	}

	priorRecord, hasPrior := store.Operation(key, ord)
	isCurrent, freshnessReason, checkErr := fileOperationCurrent(ctx, priorRecord, hasPrior, defFingerprint, basis, path)
	if checkErr != nil {
		return false, manifest.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", path, checkErr)
	}
	return isCurrent, priorRecord, freshnessReason, nil
}

// fileRecordOperation persists op's freshly committed state as its Task's
// next pending manifest record, committed only once the Task itself
// settles Done (spec §8.2/§11.3). The Basis recorded is the one observed
// before the commit: a Basis that changes during the write is drift the
// next Run must see, not state to paper over.
func (o *Output) fileRecordOperation(ctx context.Context, op fileOperation) error {
	defFingerprint := fileDefinitionFingerprint(op.path, op.contentsManaged(), op.spec.Contents, uint32(op.spec.Mode), op.basis)
	outputDigest, digestErr := pathOutputDigest(ctx, op.path)
	if digestErr != nil {
		return fmt.Errorf("evo: File %q: %w", op.path, digestErr)
	}
	rec := manifest.OperationRecord{
		Kind:                  "file",
		DefinitionFingerprint: defFingerprint,
		Basis:                 op.basis,
		Outputs:               []manifest.OutputRecord{{Kind: "file", Path: op.path, Digest: outputDigest}},
	}
	o.mu.Lock()
	o.appendManifestOperationLocked(op.taskID, rec)
	o.mu.Unlock()
	return nil
}
