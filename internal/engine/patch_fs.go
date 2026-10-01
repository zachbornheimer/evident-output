package engine

import (
	"fmt"
	"io/fs"
	"os"
)

// newDirectoryMode is the permission of a directory Patch creates for a
// new file.
const newDirectoryMode fs.FileMode = 0o755

// remover is the optional FileFS capability Patch uses to delete a file.
type remover interface{ Remove(path string) error }

// directoryMaker is the optional FileFS capability Patch uses to create the
// parent directories of a new file.
type directoryMaker interface {
	MkdirAll(path string, mode fs.FileMode) error
}

func (osFileFS) Remove(path string) error { return os.Remove(path) }

func (osFileFS) MkdirAll(path string, mode fs.FileMode) error { return os.MkdirAll(path, mode) }

func removeThrough(fsys FileFS, path string) error {
	r, ok := fsys.(remover)
	if !ok {
		return fmt.Errorf("evo: Patch cannot delete %q: the filesystem facade has no Remove", path)
	}
	return r.Remove(path)
}

func makeDirectoriesThrough(fsys FileFS, path string) error {
	m, ok := fsys.(directoryMaker)
	if !ok {
		return fmt.Errorf("evo: Patch cannot create %q: the filesystem facade has no MkdirAll", path)
	}
	return m.MkdirAll(path, newDirectoryMode)
}
