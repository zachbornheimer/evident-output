// This file owns the reconciliation of one File: claim its path, consult the
// manifest, inspect, mutate and verify, in an order where nothing waits while
// holding a claim.

package change

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/v2/internal/fs"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// reconcileFile is File's implementation (spec §11.3-11.5).
//
// Coordination (ZYS-840) is automatic and ordered so nothing waits while
// holding a claim: the cross-process manifest lock is taken first, then
// each FSPath Basis entry is observed under its own read claim, and only
// then does File claim its own path for writing — covering inspection,
// the write, and the recorded output digest.
func (r *Run) reconcileFile(ctx context.Context, id record.TaskID, spec FileSpec) error {
	if spec.Path == "" {
		return r.refuse(ErrFileSpecMissingPath)
	}
	return r.establishFile(ctx, fileOperation{taskID: id, spec: spec, path: r.resolvePath(spec.Path)})
}

// fileOperation is one File call's resolved inputs: the owning Task, the
// caller's spec, its canonical path, the Basis observed for it, and, for
// a derived state, the source it was derived from (nil for plain File).
type fileOperation struct {
	taskID      record.TaskID
	spec        FileSpec
	path        string
	basis       []freshness.BasisRecord
	derivedFrom *derivation
	// recommit forces a contents write even when the bytes on disk already
	// match: the Basis moved, so the operation runs again (CONTRACT.md §3).
	recommit bool
}

func (op fileOperation) contentsManaged() bool { return op.spec.Contents != nil }

// manifestManaged reports whether the manifest tracks this operation. A
// mode-only spec (Contents nil, Mode set) still has a managed attribute:
// consulting/recording it lets an unchanged mode-only File skip
// re-inspection on a later Run exactly like a contents-managed one does.
func (op fileOperation) manifestManaged() bool { return op.contentsManaged() || op.spec.Mode != 0 }

// plannedChanges is the path a planned op would change: its own, when the
// delta mutates it.
func (op fileOperation) plannedChanges(delta fileDelta) []string {
	if delta.mutates() {
		return []string{op.path}
	}
	return nil
}

func (op fileOperation) subject() Subject { return Subject{Kind: SubjectFile, Name: op.path} }

// establishFile claims op's path and reconciles it. op.derivedFrom, when
// set, is checked while the path is held for writing, immediately before
// the commit, so no write can land between the check and the commit.
func (r *Run) establishFile(ctx context.Context, op fileOperation) (err error) {
	defer func() {
		if err == nil {
			err = r.conflictOf(op.taskID)
		}
	}()
	target := FSResource(op.path)
	if nestedErr := checkResourceFree(ctx, target, ClaimWrite); nestedErr != nil {
		return fmt.Errorf("evo: File %q: %w", op.spec.Path, nestedErr)
	}
	if claimErr := r.claimOutput(op.taskID, op.path); claimErr != nil {
		return claimErr
	}
	defer r.barrier.Settle(op.path)

	if op.manifestManaged() {
		basis, basisErr := r.observeFileBasis(ctx, op)
		if basisErr != nil {
			return basisErr
		}
		op.basis = basis
		if op.derivedFrom != nil {
			op.basis = op.derivedFrom.recordedBasis(basis)
		}
	}
	return r.Hold(ctx, target, ClaimWrite, func(held context.Context) error {
		if op.derivedFrom != nil {
			if staleErr := op.derivedFrom.revalidate(r.host.FS(), op.path); staleErr != nil {
				return staleErr
			}
		}
		return r.commitFile(held, op)
	})
}

// commitFile reconciles op's path while File holds it for writing: the
// manifest freshness check, the live inspection, and any mutation.
func (r *Run) commitFile(ctx context.Context, op fileOperation) error {
	started := Observation{Task: op.taskID, Kind: OperationStarted, Subject: op.subject()}
	if op.manifestManaged() {
		current, prior, reason, consultErr := r.consultFile(ctx, op)
		if consultErr != nil {
			return consultErr
		}
		if current {
			r.carryForwardCurrentFile(op, prior, reason)
			return nil
		}
		started.Reason = reason
		op.recommit = op.contentsManaged() && (reason == freshness.ReasonBasisDrift || reason == freshness.ReasonPlannedInput)
	}
	r.host.Observe(started)
	return r.applyFile(ctx, op)
}

// carryForwardCurrentFile handles cross-run freshness (spec §11.4/§11.5):
// the manifest already proves this operation is current, so no live
// inspection, no write syscall, and no Effect — an unchanged File is
// silent on its second Run. The prior record still carries forward so a
// later Task settle recommits identical state. This is the "nested
// operation skipped as current" case (schema/event.v2.json), distinct from a whole
// Task skipped by a pre-definition Verify (evidence.evaluated).
func (r *Run) carryForwardCurrentFile(op fileOperation, prior freshness.OperationRecord, reason string) {
	r.host.Observe(Observation{Task: op.taskID, Kind: OperationSkippedCurrent, Subject: op.subject(), Reason: reason})
	prior.Carried = true
	r.host.AppendOperation(op.taskID, prior)
}

// applyFile inspects op's path and brings it to the desired state (spec
// §8.2), recording the Effect, verification, and manifest operation.
func (r *Run) applyFile(ctx context.Context, op fileOperation) error {
	fsys := r.host.FS()
	delta, inspectErr := r.inspectFile(fsys, op)
	if inspectErr != nil {
		return inspectErr
	}
	delta.writeNeeded = delta.writeNeeded || op.recommit
	if r.host.DryRun() {
		// Planning only: every check above already ran read-only; no
		// mutation happens and no manifest state commits (spec §8.2/§11.3).
		if op.manifestManaged() {
			r.host.PlanOperation(op.taskID, op.plannedChanges(delta)...)
		}
		r.recordFileEffectIf(op, delta.mutates())
		r.finishFile(op, delta.mutates())
		return nil
	}
	if mutateErr := r.mutateFile(fsys, op, delta); mutateErr != nil {
		r.host.FailOperation(op.taskID, freshness.OperationKindFile)
		return mutateErr
	}
	r.recordFileEffectIf(op, delta.mutates())
	if op.manifestManaged() {
		if recordErr := r.recordFileOperation(ctx, op); recordErr != nil {
			return recordErr
		}
	}
	r.finishFile(op, delta.mutates())
	return nil
}

// mutateFile applies delta to op's path and attaches the per-attribute
// verification it produced.
func (r *Run) mutateFile(fsys sysfs.FS, op fileOperation, delta fileDelta) error {
	spec, path := op.spec, op.path
	if delta.writeNeeded {
		if err := writeContents(fsys, path, spec.Contents, delta); err != nil {
			return fmt.Errorf("evo: File write %q: %w", path, err)
		}
	}
	var chmodErr error
	if delta.modeDiffers {
		chmodErr = fsys.Chmod(path, spec.Mode)
	}
	if details := fileVerificationDetails(op.contentsManaged(), spec.Mode != 0, chmodErr, spec.Path, spec.Mode); len(details) > 0 {
		r.host.AttachVerification(op.taskID, details)
	}
	if chmodErr != nil {
		return ErrFilePermissionsFailed
	}
	return nil
}

// writeContents writes contents to path at delta's permission, letting the
// real filesystem apply the umask itself to create an unmanaged-mode file.
func writeContents(fsys sysfs.FS, path string, contents []byte, delta fileDelta) error {
	if delta.ordinaryCreate {
		return sysfs.CreateOrdinary(fsys, path, contents)
	}
	return fsys.WriteAtomic(path, contents, delta.writeMode)
}

// finishFile narrates that op's operation is done.
func (r *Run) finishFile(op fileOperation, changed bool) {
	r.host.Observe(Observation{Task: op.taskID, Kind: OperationFinished, Subject: op.subject(), Changed: changed})
}

// recordFileEffectIf records op's Effect when reconciling it mutates.
func (r *Run) recordFileEffectIf(op fileOperation, mutates bool) {
	if mutates {
		r.recordEntry(op.taskID, record.NamedEntry("write", op.spec.Path))
	}
}

// recordEntry resolves id's ledger and records e there — the entry point for
// File, Exec and Patch, whose rows carry no callback, so there is no window
// between resolving and recording (compare Effect, which resolves before its
// callback runs). A Task the ledger refuses records nothing further.
func (r *Run) recordEntry(id record.TaskID, e record.LedgerEntry) {
	ledger, err := r.host.Ledger(id)
	if err != nil {
		return
	}
	ledger.Record(e)
}
