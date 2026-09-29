package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// workspaceFile is a patch target: the workspace root it was resolved
// against and its path relative to that root. A patch only edits paths
// beneath its root, and text alone cannot prove that: a symlinked
// directory inside the workspace can point anywhere.
type workspaceFile struct {
	root string
	rel  string
}

func (w workspaceFile) path() string { return filepath.Join(w.root, w.rel) }

// checkParents Lstats every directory between the root and the file
// without following a symlink. A symlinked parent fails with
// ErrPatchUnsupported (git apply: "beyond a symbolic link"); a missing or
// non-directory parent fails with ErrPatchDoesNotApply, because Patch
// never creates directories.
func (w workspaceFile) checkParents(fsys FileFS) error {
	dir := filepath.Dir(w.rel)
	if dir == "." {
		return nil
	}
	parent := w.root
	for part := range strings.SplitSeq(dir, string(filepath.Separator)) {
		parent = filepath.Join(parent, part)
		rel, _ := filepath.Rel(w.root, parent)
		info, err := fsys.Lstat(parent)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("%w: %s needs directory %s, which does not exist", ErrPatchDoesNotApply, w.rel, rel)
		case err != nil:
			return fmt.Errorf("evo: Patch inspect %q: %w", parent, err)
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: %s is beyond symbolic link %s", ErrPatchUnsupported, w.rel, rel)
		case !info.IsDir():
			return fmt.Errorf("%w: %s needs directory %s, which is not a directory", ErrPatchDoesNotApply, w.rel, rel)
		}
	}
	return nil
}
