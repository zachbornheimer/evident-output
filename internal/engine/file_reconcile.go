package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// reconcileFile is File's implementation, an Output method so it can read
// o.cfg.dryRun, the captured workspace directory, and this Run's manifest
// (spec §11.3-11.5).
//
// Coordination (ZYS-840) is automatic and ordered so nothing waits while
// holding a claim: the cross-process manifest lock is taken first, then
// each FSPath Basis entry is observed under its own read claim, and only
// then does File claim its own path for writing — covering inspection,
// the write, and the recorded output digest.
func (o *Output) reconcileFile(ctx context.Context, taskID string, spec FileSpec) error {
	if spec.Path == "" {
		return ErrFileSpecMissingPath
	}

	return o.establishFile(ctx, fileOperation{taskID: taskID, spec: spec, path: o.resolveWorkspacePath(spec.Path)})
}

// establishFile claims op's path and reconciles it. op.derivedFrom, when
// set, is checked while the path is held for writing, immediately before
// the commit, so no write can land between the check and the commit.
func (o *Output) establishFile(ctx context.Context, op fileOperation) error {
	target := FSResource(op.path)
	if nestedErr := checkResourceFree(ctx, target, resourceWrite); nestedErr != nil {
		return fmt.Errorf("evo: File %q: %w", op.spec.Path, nestedErr)
	}

	o.mu.Lock()
	claimErr := o.claimManifestOutputLocked(op.taskID, op.path)
	o.mu.Unlock()
	if claimErr != nil {
		return claimErr
	}
	defer o.outputBarrier().Settle(op.path)

	if op.manifestManaged() {
		basis, basisErr := o.fileObserveBasis(ctx, op.taskID, op.spec, op.path)
		if basisErr != nil {
			return basisErr
		}
		op.basis = basis
		if op.derivedFrom != nil {
			op.basis = op.derivedFrom.recordedBasis(basis)
		}
	}
	return o.holdResource(ctx, target, resourceWrite, func(held context.Context) error {
		if op.derivedFrom != nil {
			if staleErr := op.derivedFrom.revalidate(o.fileFS(), op.path); staleErr != nil {
				return staleErr
			}
		}
		return o.commitFile(held, op)
	})
}

// fileOperation is one File call's resolved inputs: the owning Task, the
// caller's spec, its canonical path, the Basis observed for it, and, for
// a derived state, the source it was derived from (nil for plain File).
type fileOperation struct {
	taskID      string
	spec        FileSpec
	path        string
	basis       []freshness.BasisRecord
	derivedFrom *derivation
}

func (op fileOperation) contentsManaged() bool { return op.spec.Contents != nil }

// manifestManaged reports whether the manifest tracks this operation. A
// mode-only spec (Contents nil, Mode set) still has a managed attribute:
// consulting/recording it lets an unchanged mode-only File skip
// re-inspection on a later Run exactly like a contents-managed one does.
func (op fileOperation) manifestManaged() bool { return op.contentsManaged() || op.spec.Mode != 0 }

// commitFile reconciles op's path while File holds it for writing: the
// manifest freshness check, the live inspection, and any mutation.
func (o *Output) commitFile(ctx context.Context, op fileOperation) error {
	started := map[string]any{"kind": "file", "path": op.path}
	if op.manifestManaged() {
		current, prior, reason, consultErr := o.fileConsultManifest(ctx, op.taskID, op.spec, op.path, op.basis)
		if consultErr != nil {
			return consultErr
		}
		if current {
			o.carryForwardCurrentFile(op, prior, reason)
			return nil
		}
		started["reason"] = reason
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventOperationStarted, op.taskID, started)
	o.mu.Unlock()
	return o.applyFile(ctx, op)
}

// carryForwardCurrentFile handles cross-run freshness (spec §11.4/§11.5):
// the manifest already proves this operation is current, so no live
// inspection, no write syscall, and no Effect — an unchanged File is
// silent on its second Run. The prior record still carries forward so a
// later Task settle recommits identical state. This is the "nested
// operation skipped as current" case (spec §38), distinct from a whole
// Task skipped by a pre-definition Verify (evidence.evaluated).
func (o *Output) carryForwardCurrentFile(op fileOperation, prior freshness.OperationRecord, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventOperationSkippedCurrent, op.taskID, map[string]any{
		"kind": "file", "path": op.path, "reason": reason,
	})
	if !o.cfg.dryRun {
		o.appendManifestOperationLocked(op.taskID, prior)
	}
}

// fileDelta is what reconciling one File must change on disk.
type fileDelta struct {
	exists      bool
	writeNeeded bool
	modeDiffers bool
	// writeMode is the permission a content write must leave: the managed
	// Mode, else the existing file's own, else unmanagedCreateMode.
	writeMode fs.FileMode
	// ordinaryCreate is a new file whose mode is unmanaged: the umask
	// decides its permission.
	ordinaryCreate bool
}

func (d fileDelta) mutates() bool { return d.writeNeeded || d.modeDiffers }

// applyFile inspects op's path and brings it to the desired state (spec
// §8.2), recording the Effect, verification, and manifest operation.
func (o *Output) applyFile(ctx context.Context, op fileOperation) error {
	fsys := o.fileFS()
	delta, inspectErr := o.inspectFile(fsys, op)
	if inspectErr != nil {
		return inspectErr
	}
	if o.DryRun() {
		// Planning only: every check above already ran read-only; no
		// mutation happens and no manifest state commits (spec §8.2/§11.3).
		o.recordFileEffectIf(op, delta.mutates())
		o.emitFileFinished(op, delta.mutates())
		return nil
	}
	if mutateErr := o.mutateFile(fsys, op, delta); mutateErr != nil {
		return mutateErr
	}
	o.recordFileEffectIf(op, delta.mutates())
	if op.manifestManaged() {
		if recordErr := o.fileRecordOperation(ctx, op); recordErr != nil {
			return recordErr
		}
	}
	o.emitFileFinished(op, delta.mutates())
	return nil
}

// inspectFile observes op's path read-only and reports what reconciling
// it must change.
func (o *Output) inspectFile(fsys FileFS, op fileOperation) (fileDelta, error) {
	spec, path := op.spec, op.path
	info, exists, statErr := inspectFilePath(fsys, path)
	if statErr != nil {
		return fileDelta{}, statErr
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventTrackedResourceObserved, op.taskID, map[string]any{
		"kind": "file", "path": path, "exists": exists,
	})
	o.mu.Unlock()

	if !exists && !op.contentsManaged() {
		return fileDelta{}, fmt.Errorf("%w: %s", ErrFileUnmanagedContentsMissing, path)
	}
	writeNeeded, readErr := fileNeedsContentWrite(fsys, path, exists, op.contentsManaged(), spec.Contents)
	if readErr != nil {
		return fileDelta{}, fmt.Errorf("evo: File inspect %q: %w", path, readErr)
	}
	modeDiffers := spec.Mode != 0 && (!exists || info.Mode().Perm() != spec.Mode.Perm())
	return fileDelta{
		exists:         exists,
		writeNeeded:    writeNeeded,
		modeDiffers:    modeDiffers,
		writeMode:      contentWriteMode(spec.Mode, info, exists),
		ordinaryCreate: !exists && spec.Mode == 0,
	}, nil
}

// inheritedModeBits are the mode bits an unmanaged rewrite carries over:
// the permissions and setuid, setgid, and sticky, which Perm() alone
// would drop.
const inheritedModeBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// ownerRead is the permission bit readExisting adds to open a file whose
// mode denies even its owner a read.
const ownerRead fs.FileMode = 0o400

// contentWriteMode is the permission a content write leaves: the managed
// mode when there is one, otherwise the existing file's own (an unmanaged
// rewrite never changes it), otherwise ordinary creation semantics.
func contentWriteMode(managed fs.FileMode, info fs.FileInfo, exists bool) fs.FileMode {
	switch {
	case managed != 0:
		return managed
	case exists:
		return info.Mode() & inheritedModeBits
	default:
		return unmanagedCreateMode
	}
}

// mutateFile applies delta to op's path and attaches the per-attribute
// verification it produced.
func (o *Output) mutateFile(fsys FileFS, op fileOperation, delta fileDelta) error {
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
		o.mu.Lock()
		o.attachVerificationLocked(op.taskID, details)
		o.mu.Unlock()
	}
	if chmodErr != nil {
		return ErrFilePermissionsFailed
	}
	return nil
}

// writeContents writes contents to path at delta's permission, letting the
// real filesystem apply the umask itself to create an unmanaged-mode file.
func writeContents(fsys FileFS, path string, contents []byte, delta fileDelta) error {
	if delta.ordinaryCreate {
		return createOrdinary(fsys, path, contents)
	}
	return fsys.WriteAtomic(path, contents, delta.writeMode)
}

// inspectFilePath Lstats path without following a symlink and rejects
// anything File cannot own: a symlink, a directory, or another
// non-regular type. A missing path is not an error.
func inspectFilePath(fsys FileFS, path string) (info fs.FileInfo, exists bool, err error) {
	info, statErr := fsys.Lstat(path)
	switch {
	case statErr == nil && info.Mode()&fs.ModeSymlink != 0:
		return nil, false, fmt.Errorf("%w: %s", ErrFilePathIsSymlink, path)
	case statErr == nil && info.IsDir():
		return nil, false, fmt.Errorf("%w: %s is a directory", ErrFilePathTypeMismatch, path)
	case statErr == nil && !info.Mode().IsRegular():
		return nil, false, fmt.Errorf("%w: %s", ErrFilePathTypeMismatch, path)
	case statErr != nil && !errors.Is(statErr, fs.ErrNotExist):
		return nil, false, fmt.Errorf("evo: File inspect %q: %w", path, statErr)
	}
	return info, statErr == nil, nil
}

// emitFileFinished emits op's operation-finished event.
func (o *Output) emitFileFinished(op fileOperation, changed bool) {
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventOperationFinished, op.taskID, map[string]any{"kind": "file", "path": op.path, "changed": changed})
	o.mu.Unlock()
}

// recordFileEffectIf records op's Effect when reconciling it mutates.
func (o *Output) recordFileEffectIf(op fileOperation, mutates bool) {
	if mutates {
		o.recordFileEffect(op.taskID, op.spec.Path)
	}
}

// recordFileEffect records File's planned (dry-run) or committed (applied)
// Effect under taskID's own ledger section (spec §8.2/§27/§51) — the same
// Plan/Changes routing evo.Effect and evo.Exec already use.
func (o *Output) recordFileEffect(taskID, displayPath string) {
	o.recordLedgerEntry(taskID, record.NamedEntry("write", displayPath))
}
