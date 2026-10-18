// This file owns how a File reports a failed filesystem operation: once, in
// one clause, naming the path relative to the workspace.

package change

import (
	"path/filepath"
	"strings"

	sysfs "github.com/zachbornheimer/evident-output/v2/internal/fs"
)

// fileFailure is a filesystem operation's failure as a File reports it:
// "out/a.html: permission denied". cause stays reachable for errors.Is.
type fileFailure struct {
	path   string
	reason string
	cause  error
}

func (f *fileFailure) Error() string { return f.path + ": " + f.reason }
func (f *fileFailure) Unwrap() error { return f.cause }

// fileFailureOf reports err, a failure of the operation on path, as a
// fileFailure when the operating system raised it. Any other error already
// says what is wrong and passes through unchanged.
func (r *Run) fileFailureOf(path string, err error) error {
	reason, raised := sysfs.FailureReason(err)
	if !raised {
		return err
	}
	return &fileFailure{path: r.workspaceRelative(path), reason: reason, cause: err}
}

// workspaceRelative is path relative to the Run's workspace, or path itself
// when it lies outside it.
func (r *Run) workspaceRelative(path string) string {
	rel, err := filepath.Rel(r.Workspace(), path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
