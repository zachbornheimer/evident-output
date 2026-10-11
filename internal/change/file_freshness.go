// This file owns what File asks of freshness: claiming the output path, then
// observing Basis, consulting the manifest and recording the operation.

package change

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// ErrFileConflictingProducer is returned when two Tasks in one Run both
// claim the same canonical File output path (spec §8.3/§11.4: "more than
// one producing operation claiming the same canonical file output in one
// Run is a conflict").
var ErrFileConflictingProducer = errors.New("evo: File output path already claimed by another Task in this Run")

// claimOutput records that id is the producing Task for canonical path, or
// reports ErrFileConflictingProducer when a different Task already claimed
// it in this Run (spec §8.3/§11.4). It opens path's freshness gate, so a
// later Task reading path as a Basis input waits for the producer.
func (r *Run) claimOutput(id record.TaskID, path string) error {
	r.outputsMu.Lock()
	defer r.outputsMu.Unlock()
	return r.claimOutputLocked(id, path)
}

func (r *Run) claimOutputLocked(id record.TaskID, path string) error {
	if r.outputs == nil {
		r.outputs = make(map[string]record.TaskID)
	}
	if owner, claimed := r.outputs[path]; claimed && owner != id {
		return fmt.Errorf("%w: %s", ErrFileConflictingProducer, path)
	}
	r.outputs[path] = id
	r.barrier.Open(path)
	return nil
}

// ObserveBasis fingerprints basis for one tracked operation (spec §11.1)
// under read claims on each path (ZYS-840).
func (r *Run) ObserveBasis(ctx context.Context, basis []freshness.Fingerprint) ([]freshness.BasisRecord, error) {
	return freshness.ObserveBasis(ctx, r, basis)
}

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

// observeFileBasis opens this Run's manifest — acquiring its cross-process
// lock before File claims any resource — and observes the spec's Basis under
// read claims (see ObserveBasis).
func (r *Run) observeFileBasis(ctx context.Context, op fileOperation) ([]freshness.BasisRecord, error) {
	store, openErr := r.host.Manifest(ctx)
	if openErr != nil {
		return nil, fmt.Errorf("evo: File %q: %w", op.path, openErr)
	}
	r.host.NoteManifestWarning(store.Warning())

	basis, observeErr := r.ObserveBasis(ctx, op.spec.Basis)
	if observeErr != nil {
		return nil, fmt.Errorf("evo: File %q: %w", op.path, observeErr)
	}
	r.host.Observe(Observation{Task: op.taskID, Kind: BasisFingerprinted, Subject: op.subject(), Count: len(basis)})
	return basis, nil
}

// consultFile resolves this File call's prior operation record (if any) and
// reports whether it is still current against the observed basis (spec
// §11.4/§11.5), plus the freshness reason (spec §38: "Basis drift" vs
// "tracked output drift" vs no prior record must be distinguishable). prior
// is always returned so the caller can carry it forward unchanged on a
// current hit.
func (r *Run) consultFile(ctx context.Context, op fileOperation) (current bool, prior freshness.OperationRecord, reason string, err error) {
	store, openErr := r.host.Manifest(ctx)
	if openErr != nil {
		return false, freshness.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", op.path, openErr)
	}
	key, ordinal, ok := r.host.TaskManifest(op.taskID)
	if !ok {
		return false, freshness.OperationRecord{}, "", graph.ErrNoTaskContext
	}
	verdict, consultErr := fileCallFor(op.path, op.spec, op.basis).Consult(ctx, store, key, ordinal)
	if consultErr != nil {
		return false, freshness.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", op.path, consultErr)
	}
	return verdict.Current, verdict.Prior, verdict.Reason, nil
}

// recordFileOperation persists op's freshly committed state as its Task's
// next pending manifest record, committed only once the Task itself settles
// Done (spec §8.2/§11.3).
func (r *Run) recordFileOperation(ctx context.Context, op fileOperation) error {
	rec, recordErr := fileCallFor(op.path, op.spec, op.basis).Record(ctx)
	if recordErr != nil {
		return fmt.Errorf("evo: File %q: %w", op.path, recordErr)
	}
	r.host.AppendOperation(op.taskID, rec)
	return nil
}
