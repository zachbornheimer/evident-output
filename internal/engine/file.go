package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// getwd is the facade File's relative-path resolution reads the process's
// current working directory through, instead of os.Getwd directly (facade
// rule).
var getwd = os.Getwd

// FileSpec declares one managed-state file resource (spec §8/§8.1).
// Constructing a FileSpec performs no I/O; File performs the operation.
type FileSpec struct {
	// Path is required. A relative Path resolves against the Run's
	// workspace directory captured once at Run start.
	Path string
	// Contents == nil means contents are not managed. Non-nil (including
	// empty) means the desired file contents are exactly Contents.
	Contents []byte
	// Mode == 0 means mode is unmanaged in this v1 shorthand.
	Mode fs.FileMode
	// Basis lists additional Fingerprint inputs whose change invalidates
	// this operation's prior record even when Contents/Mode alone would
	// look unchanged (spec §11.4).
	Basis []fingerprint.Fingerprint
}

// File-specific misuse/usage errors (spec §8.1).
var (
	// ErrFileSpecMissingPath is returned when FileSpec.Path is empty.
	ErrFileSpecMissingPath = errors.New("evo: FileSpec.Path is required")
	// ErrFileUnmanagedContentsMissing is returned when Path does not exist
	// and Contents is nil: Evo cannot invent contents merely to apply mode.
	ErrFileUnmanagedContentsMissing = errors.New("evo: File cannot create a missing path with unmanaged Contents")
	// ErrFilePathIsSymlink is returned when an existing symlink occupies
	// Path — File rejects it rather than following it implicitly.
	ErrFilePathIsSymlink = errors.New("evo: File rejects an existing symlink at Path")
	// ErrFilePathTypeMismatch is returned when Path exists but is a
	// directory, device, or other non-regular-file type.
	ErrFilePathTypeMismatch = errors.New("evo: File Path exists as a non-regular-file type")
)

// File declares/reconciles one managed-state file resource against spec
// (spec §8.2's reconciliation algorithm): inspect the existing path safely,
// no-op when every managed attribute already matches, otherwise write
// replacement contents and/or chmod, then re-verify. ctx must come from a
// Task's Define callback (see taskScope) — File returns ErrNoTaskContext or
// ErrTaskClosed otherwise.
func File(ctx context.Context, spec FileSpec) error {
	task, err := taskScope(ctx)
	if err != nil {
		return err
	}
	return task.out.reconcileFile(ctx, task.id, spec)
}

// reconcileFile is File's implementation, an Output method so it can read
// o.cfg.dryRun, the captured workspace directory, and this Run's manifest
// (spec §11.3-11.5).
func (o *Output) reconcileFile(ctx context.Context, taskID string, spec FileSpec) error {
	if spec.Path == "" {
		return ErrFileSpecMissingPath
	}
	if err := ctx.Err(); err != nil {
		wrapped := fmt.Errorf("evo: File %q: %w", spec.Path, err)
		// A cancelled Run must never look like it silently succeeded: File
		// refusing a promised mutation because its context is already done
		// is exactly the kind of caller-visible outcome Output.Err() exists
		// to surface (the same first-recorded-issue channel Key/duplicate/
		// limit misuse already reports through). recordMisuse assumes its
		// caller already holds o.mu (every other call site in this package
		// is itself already inside a locked section).
		o.mu.Lock()
		o.recordMisuse(wrapped)
		o.mu.Unlock()
		return wrapped
	}

	path := o.resolveWorkspacePath(spec.Path)
	contentsManaged := spec.Contents != nil
	// A mode-only spec (Contents nil, Mode set) still has a managed
	// attribute the manifest must track: consulting/recording it lets an
	// unchanged mode-only File skip re-inspection on a later Run exactly
	// like a contents-managed one does.
	manifestManaged := contentsManaged || spec.Mode != 0

	o.mu.Lock()
	claimErr := o.claimManifestOutputLocked(taskID, path)
	o.mu.Unlock()
	if claimErr != nil {
		return claimErr
	}
	defer o.settleOutputBarrier(path)

	if manifestManaged {
		current, prior, reason, err := o.fileConsultManifest(ctx, taskID, spec, path)
		if err != nil {
			return err
		}
		if current {
			// Cross-run freshness (spec §11.4/§11.5): the manifest already
			// proves this operation is current, so no live inspection, no
			// write syscall, and no Effect — an unchanged File is silent on
			// its second Run. The prior record still carries forward so a
			// later Task settle recommits identical state. This is the
			// "nested operation skipped as current"/"upstream revalidated
			// with identical output" case (spec §38), distinct from a whole
			// Task skipped by a pre-definition Verify (evidence.evaluated).
			o.mu.Lock()
			o.emitWireEventLocked(wire.EventOperationSkippedCurrent, taskID, map[string]any{
				"kind": "file", "path": path, "reason": reason,
			})
			o.mu.Unlock()
			if !o.DryRun() {
				o.mu.Lock()
				o.appendManifestOperationLocked(taskID, prior)
				o.mu.Unlock()
			}
			return nil
		}
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventOperationStarted, taskID, map[string]any{
			"kind": "file", "path": path, "reason": reason,
		})
		o.mu.Unlock()
	} else {
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventOperationStarted, taskID, map[string]any{"kind": "file", "path": path})
		o.mu.Unlock()
	}

	info, statErr := os.Lstat(path)
	switch {
	case statErr == nil && info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: %s", ErrFilePathIsSymlink, path)
	case statErr == nil && info.IsDir():
		return fmt.Errorf("%w: %s is a directory", ErrFilePathTypeMismatch, path)
	case statErr == nil && !info.Mode().IsRegular():
		return fmt.Errorf("%w: %s", ErrFilePathTypeMismatch, path)
	case statErr != nil && !os.IsNotExist(statErr):
		return fmt.Errorf("evo: File inspect %q: %w", path, statErr)
	}
	exists := statErr == nil
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventTrackedResourceObserved, taskID, map[string]any{
		"kind": "file", "path": path, "exists": exists,
	})
	o.mu.Unlock()

	if !exists && !contentsManaged {
		return fmt.Errorf("%w: %s", ErrFileUnmanagedContentsMissing, path)
	}

	needsWrite, err := fileNeedsContentWrite(path, exists, contentsManaged, spec.Contents)
	if err != nil {
		return fmt.Errorf("evo: File inspect %q: %w", path, err)
	}
	modeDiffers := spec.Mode != 0 && (!exists || info.Mode().Perm() != spec.Mode.Perm())
	mutates := needsWrite || modeDiffers

	if o.DryRun() {
		// Planning only: every check above already ran read-only; no
		// mutation happens and no manifest state commits (spec §8.2/§11.3).
		if mutates {
			o.recordFileEffect(taskID, spec.Path)
		}
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventOperationFinished, taskID, map[string]any{"kind": "file", "path": path, "changed": mutates})
		o.mu.Unlock()
		return nil
	}

	if needsWrite {
		if err := writeFileAtomic(path, spec.Contents, contentCreateMode(exists, spec.Mode)); err != nil {
			return fmt.Errorf("evo: File write %q: %w", path, err)
		}
	}

	if modeDiffers {
		if err := os.Chmod(path, spec.Mode); err != nil {
			return fmt.Errorf("evo: File %q: contents already satisfied, permissions failed: %w", path, err)
		}
	}

	if mutates {
		o.recordFileEffect(taskID, spec.Path)
	}
	if manifestManaged {
		if err := o.fileRecordOperation(ctx, taskID, spec, path, contentsManaged); err != nil {
			return err
		}
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventOperationFinished, taskID, map[string]any{"kind": "file", "path": path, "changed": mutates})
	o.mu.Unlock()
	return nil
}

// recordFileEffect records File's planned (dry-run) or committed (applied)
// Effect under taskID's own ledger section (spec §8.2/§27/§51) — the same
// Plan/Changes routing TaskHandle's named mutation verbs already use.
func (o *Output) recordFileEffect(taskID, displayPath string) {
	(&TaskHandle{out: o, id: taskID}).RecordName("write", displayPath)
}

// fileConsultManifest resolves this File call's prior operation record (if
// any) and reports whether it is still current (spec §11.4/§11.5), plus the
// freshness reason (spec §38: "Basis drift" vs "tracked output drift" vs no
// prior record must be distinguishable). prior is always returned so the
// caller can carry it forward unchanged on a current hit.
func (o *Output) fileConsultManifest(ctx context.Context, taskID string, spec FileSpec, path string) (current bool, prior manifest.OperationRecord, reason string, err error) {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return false, manifest.OperationRecord{}, "", fmt.Errorf("evo: File %q: %w", path, openErr)
	}
	o.emitManifestWarningOnce(store.Warning())

	basis, err := basisRecordsFrom(ctx, spec.Basis)
	if err != nil {
		return false, manifest.OperationRecord{}, "", err
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventBasisFingerprinted, taskID, map[string]any{
		"path": path, "count": len(basis),
	})
	o.mu.Unlock()
	contentsManaged := spec.Contents != nil
	defFingerprint := fileDefinitionFingerprint(path, contentsManaged, spec.Contents, uint32(spec.Mode), basis)

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

// fileRecordOperation persists this File call's freshly observed operation
// state as taskID's next pending manifest record, committed only once the
// Task itself settles Done (spec §8.2/§11.3).
func (o *Output) fileRecordOperation(ctx context.Context, taskID string, spec FileSpec, path string, contentsManaged bool) error {
	basis, err := basisRecordsFrom(ctx, spec.Basis)
	if err != nil {
		return err
	}
	defFingerprint := fileDefinitionFingerprint(path, contentsManaged, spec.Contents, uint32(spec.Mode), basis)
	outputDigest, err := pathOutputDigest(ctx, path)
	if err != nil {
		return fmt.Errorf("evo: File %q: %w", path, err)
	}
	rec := manifest.OperationRecord{
		Kind:                  "file",
		DefinitionFingerprint: defFingerprint,
		Basis:                 basis,
		Outputs:               []manifest.OutputRecord{{Kind: "file", Path: path, Digest: outputDigest}},
	}
	o.mu.Lock()
	o.appendManifestOperationLocked(taskID, rec)
	o.mu.Unlock()
	return nil
}

// DryRun reports whether this Output is configured for dry-run/preview
// tense — the same flag TaskHandle mutation verbs already render by.
func (o *Output) DryRun() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.dryRun
}

// resolveWorkspacePath resolves a possibly-relative FileSpec.Path against
// the workspace directory captured once for this Output (spec §8.1: "the
// Run's workspace directory captured once at Run start; changing process
// CWD later does not retarget an operation").
func (o *Output) resolveWorkspacePath(path string) string {
	return resolvePathAgainst(o.workspaceDirLocked(), path)
}

// resolvePathAgainst resolves path against base: an absolute path is
// cleaned and returned as-is (base never applies), a relative path
// (including empty, which resolves to base itself) joins base. Shared by
// File's workspace-relative Path (resolveWorkspacePath) and Exec's
// dir-relative Outputs (spec §8.4).
func resolvePathAgainst(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

// workspaceDirLocked lazily captures and caches the process working
// directory the first time any operation needs it, so every relative path
// in this Run resolves against the same snapshot even if the process CWD
// later changes.
func (o *Output) workspaceDirLocked() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.workspaceDir == "" {
		dir, err := getwd()
		if err != nil {
			dir = "."
		}
		o.workspaceDir = dir
	}
	return o.workspaceDir
}

// fileNeedsContentWrite reports whether Contents must be (re)written for
// the file to match spec: unmanaged Contents never triggers a write; a
// missing file with managed Contents always does; an existing file needs
// one only when its current bytes differ from the desired ones.
func fileNeedsContentWrite(path string, exists, contentsManaged bool, desired []byte) (bool, error) {
	if !contentsManaged {
		return false, nil
	}
	if !exists {
		return true, nil
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(current, desired), nil
}

// contentCreateMode is the mode writeFileAtomic creates a new file with:
// spec.Mode when the caller manages it, otherwise the platform's ordinary
// file-creation semantics (0666 subject to umask) so an unmanaged-mode
// create never silently claims a mode it was never asked to manage (spec
// §8.1).
func contentCreateMode(exists bool, specMode fs.FileMode) fs.FileMode {
	if specMode != 0 {
		return specMode
	}
	return 0o666
}

// writeFileAtomic writes contents to a temp file beside path and renames it
// into place — the same atomic-replace contract the manifest store's
// writeAtomic uses — so a reader never observes a partially written file.
func writeFileAtomic(path string, contents []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".evo-file-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fsync temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file %q: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %q to %q: %w", tmpPath, path, err)
	}
	return nil
}
