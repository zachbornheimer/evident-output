package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/fingerprint"
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
	// ErrFilePermissionsFailed is returned when contents already matched
	// (or were freshly written) but the chmod that would have brought Mode
	// into line failed. Deliberately terse — "failed: permissions" alone,
	// not "evo: File %q: ..." — because a Task's Define callback typically
	// returns this error unwrapped, and its Error() text becomes the
	// task's own failure headline verbatim (spec §8.2's worked example:
	// "✗ write plist  failed: permissions"); the operation, path, and
	// underlying syscall error are the failing VerificationDetail's own
	// Facts, attached to the task before this error is returned, not
	// repeated in the headline.
	ErrFilePermissionsFailed = errors.New("failed: permissions")
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

// DryRun reports whether this Output is configured for dry-run/preview
// tense — the same flag evo.Effect and TaskHandle.Record render by.
func (o *Output) DryRun() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.dryRun
}

// fileFS returns the facade File performs filesystem I/O through — real
// os calls in ordinary use, a scripted fake under test (facade rule).
func (o *Output) fileFS() FileFS {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.fileFS
}

// fileVerificationDetails builds this reconcile's per-attribute evidence
// (spec §2/§8.2): one entry per attribute File actually manages, in the
// order it inspected them. A managed attribute that reconciled cleanly is
// Satisfied with no Facts; permissions' chmod failure is VerificationError
// with the Facts spec §8.2's worked example nests under it — the operation
// (error), the target (path), and the mode that failed to apply.
func fileVerificationDetails(contentsManaged, modeManaged bool, chmodErr error, displayPath string, mode fs.FileMode) []core.VerificationDetail {
	var details []core.VerificationDetail
	if contentsManaged {
		details = append(details, core.VerificationDetail{Name: "contents", Status: core.VerificationSatisfied})
	}
	switch {
	case !modeManaged:
		// Mode unmanaged: no "permissions" attribute to report on at all.
	case chmodErr != nil:
		details = append(details, core.VerificationDetail{
			Name:   "permissions",
			Status: core.VerificationError,
			Facts: []core.Fact{
				{Name: "error", Value: chmodErr.Error()},
				{Name: "path", Value: displayPath},
				{Name: "mode", Value: fmt.Sprintf("%#o", mode.Perm())},
			},
		})
	default:
		details = append(details, core.VerificationDetail{Name: "permissions", Status: core.VerificationSatisfied})
	}
	return details
}

// resolveWorkspacePath resolves a possibly-relative FileSpec.Path against
// the workspace directory captured once for this Output (spec §8.1: "the
// Run's workspace directory captured once at Run start; changing process
// CWD later does not retarget an operation").
func (o *Output) resolveWorkspacePath(path string) string {
	return resolvePathAgainst(o.workspace(), path)
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

// workspace lazily captures and caches the process working
// directory the first time any operation needs it, so every relative path
// in this Run resolves against the same snapshot even if the process CWD
// later changes.
func (o *Output) workspace() string {
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
func fileNeedsContentWrite(fsys FileFS, path string, exists, contentsManaged bool, desired []byte) (bool, error) {
	if !contentsManaged {
		return false, nil
	}
	if !exists {
		return true, nil
	}
	current, err := fsys.ReadFile(path)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(current, desired), nil
}

// defaultCreateMode asks writeFileAtomic for ordinary file-creation
// semantics: 0666 less the process umask, applied by the kernel.
const defaultCreateMode fs.FileMode = 0

const (
	// createPermissionDefault opens a defaultCreateMode temp file: 0666,
	// masked by the umask.
	createPermissionDefault fs.FileMode = 0o666
	// createPermissionPrivate opens every other temp file owner-only until
	// its explicit chmod.
	createPermissionPrivate fs.FileMode = 0o600
	// tempNameAttempts bounds createTempFile's search for an unused name.
	tempNameAttempts = 10000
)

// writeFileAtomic writes contents to a temp file beside path and renames it
// into place — the same atomic-replace contract the manifest store's
// writeAtomic uses — so a reader never observes a partially written file.
// mode is the exact permission the file ends with; defaultCreateMode
// leaves it to the umask instead (spec §8.1: unmanaged mode is never
// claimed).
func writeFileAtomic(path string, contents []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	perm := createPermissionPrivate
	if mode == defaultCreateMode {
		perm = createPermissionDefault
	}
	tmp, err := createTempFile(dir, perm)
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := writeAndClose(tmp, contents); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if mode != defaultCreateMode {
		if err := os.Chmod(tmpPath, mode); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("chmod temp file %q: %w", tmpPath, err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %q to %q: %w", tmpPath, path, err)
	}
	return nil
}

// createTempFile creates a new, uniquely named file in dir opened with
// perm, which (unlike os.CreateTemp's fixed 0600) the umask then masks.
func createTempFile(dir string, perm fs.FileMode) (*os.File, error) {
	for range tempNameAttempts {
		name := filepath.Join(dir, ".evo-file-"+strconv.FormatUint(rand.Uint64(), 36)+".tmp")
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("no unused temp file name after %d attempts", tempNameAttempts)
}

// writeAndClose writes contents to f, fsyncs it, and closes it.
func writeAndClose(f *os.File, contents []byte) error {
	if _, err := f.Write(contents); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp file %q: %w", f.Name(), err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync temp file %q: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file %q: %w", f.Name(), err)
	}
	return nil
}
