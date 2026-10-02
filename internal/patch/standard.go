package patch

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// permissionMask keeps a diff's mode to rwx bits: a diff never grants
// setuid, setgid, or sticky.
const permissionMask = fs.ModePerm

// touched is every workspace path f reads or writes.
func (f File) touched() []string {
	if f.From != "" {
		return []string{f.From, f.Path}
	}
	return []string{f.Path}
}

// standardFile judges h as one of the standard forms: modify, create,
// delete, rename, or mode change.
func (h header) standardFile() (File, error) {
	oldPath, newPath := h.sidePaths()
	if h.rename && h.from != "" && h.to != "" {
		oldPath, newPath = h.from, h.to
	}
	switch {
	case h.binary:
		return File{}, fmt.Errorf("%w: %s", ErrBinaryUnsupported, newPath)
	case h.copied:
		return File{}, fmt.Errorf("%w: %s -> %s", ErrRenameUnsupported, oldPath, newPath)
	}
	mode, err := regularFileMode(h.mode)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", newPath, err)
	}
	mode &= permissionMask
	if h.delete || newPath == devNull {
		target, pathErr := safeWorkspacePath(oldPath)
		return File{Path: target, Delete: true, Hunks: h.hunks}, pathErr
	}
	create := h.create || oldPath == devNull
	renamed := !create && oldPath != "" && oldPath != newPath
	if renamed && !h.rename {
		return File{}, fmt.Errorf("%w: %s -> %s", ErrRenameUnsupported, oldPath, newPath)
	}
	target, err := safeWorkspacePath(newPath)
	if err != nil {
		return File{}, err
	}
	file := File{Path: target, Create: create, Mode: mode, Hunks: h.hunks}
	if renamed {
		if file.From, err = safeWorkspacePath(oldPath); err != nil {
			return File{}, err
		}
	}
	return file, nil
}

// safeWorkspacePath is workspacePath with ErrUnsafePath for every path a
// patch must not touch.
func safeWorkspacePath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: empty path", ErrMalformed)
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("%w: NUL byte in path %q", ErrUnsafePath, raw)
	}
	if path.IsAbs(raw) || filepath.IsAbs(raw) {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafePath, raw)
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: path %q escapes the patch root", ErrUnsafePath, raw)
	}
	for part := range strings.SplitSeq(clean, "/") {
		if strings.EqualFold(part, ".git") {
			return "", fmt.Errorf("%w: %q is inside a .git directory", ErrUnsafePath, raw)
		}
	}
	return filepath.FromSlash(clean), nil
}
