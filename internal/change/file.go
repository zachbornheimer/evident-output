// This file owns File: one managed-state file reconciled against its desired
// contents and mode (spec §8.2).

package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

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
	Basis []freshness.Fingerprint
}

// File-specific misuse/usage errors (spec §8.1).
var (
	// ErrFileSpecMissingPath is returned when FileSpec.Path is empty.
	ErrFileSpecMissingPath = fmt.Errorf("%w: FileSpec.Path is required", graph.ErrInvalidConfig)
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
	ErrFilePermissionsFailed = errors.New(record.FailedAttributeText(attributePermissions))
)

// attributePermissions names the mode attribute in a File's per-attribute
// verification evidence.
const attributePermissions = "permissions"

// File declares/reconciles one managed-state file resource against spec
// (spec §8.2's reconciliation algorithm): inspect the existing path safely,
// no-op when every managed attribute already matches, otherwise write
// replacement contents and/or chmod, then re-verify. ctx must come from a
// Task's Define callback (see graph.ScopeOf) — File returns ErrNoTaskContext
// or ErrTaskClosed otherwise.
func File(ctx context.Context, spec FileSpec) error {
	scope, beginErr := graph.BeginOperation[*Run](ctx, fmt.Sprintf("File %q", spec.Path))
	if beginErr != nil {
		return beginErr
	}
	return scope.Owner().reconcileFile(ctx, scope.Task(), spec)
}

// fileVerificationDetails builds this reconcile's per-attribute evidence
// (spec §2/§8.2): one entry per attribute File actually manages, in the
// order it inspected them. A managed attribute that reconciled cleanly is
// Satisfied with no Facts; permissions' chmod failure is VerificationError
// with the Facts spec §8.2's worked example nests under it — the operation
// (error), the target (path), and the mode that failed to apply.
func fileVerificationDetails(contentsManaged, modeManaged bool, chmodErr error, displayPath string, mode fs.FileMode) []record.VerificationDetail {
	var details []record.VerificationDetail
	if contentsManaged {
		details = append(details, record.VerificationDetail{Name: "contents", Status: record.VerificationSatisfied})
	}
	switch {
	case !modeManaged:
		// Mode unmanaged: no "permissions" attribute to report on at all.
	case chmodErr != nil:
		details = append(details, record.VerificationDetail{
			Name:    attributePermissions,
			Status:  record.VerificationError,
			Failure: ErrFilePermissionsFailed,
			Facts: []record.Fact{
				{Name: "error", Value: chmodErr.Error()},
				{Name: "path", Value: displayPath},
				{Name: "mode", Value: fmt.Sprintf("%#o", mode.Perm())},
			},
		})
	default:
		details = append(details, record.VerificationDetail{Name: attributePermissions, Status: record.VerificationSatisfied})
	}
	return details
}

// resolvePath resolves a possibly-relative path against the workspace
// directory captured once for this Run (spec §8.1: "the Run's workspace
// directory captured once at Run start; changing process CWD later does not
// retarget an operation").
func (r *Run) resolvePath(path string) string {
	return resolvePathAgainst(r.Workspace(), path)
}

// resolvePathAgainst resolves path against base: an absolute path is
// cleaned and returned as-is (base never applies), a relative path
// (including empty, which resolves to base itself) joins base. Shared by
// File's workspace-relative Path and Exec's dir-relative Outputs (spec §8.4).
func resolvePathAgainst(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}
