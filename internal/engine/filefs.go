package engine

import (
	"io/fs"
	"os"
)

// FileFS is the facade evo.File performs every filesystem inspection and
// mutation through, instead of the os package directly (facade rule): the
// real osFileFS backs every production Output; a test injects a fake to
// script a filesystem outcome (e.g. a chmod failure) deterministically,
// without depending on a real disk's permission behavior.
type FileFS interface {
	// Lstat inspects path without following a symlink — File's own
	// existence/type check (spec §8.2).
	Lstat(path string) (fs.FileInfo, error)
	// ReadFile reads path's current contents for File's content-diff check.
	ReadFile(path string) ([]byte, error)
	// WriteAtomic replaces path's contents with contents at mode, atomically
	// (spec §8.2's "write replacement contents") — never a partial write a
	// concurrent reader could observe.
	WriteAtomic(path string, contents []byte, mode fs.FileMode) error
	// Chmod sets path's permission bits.
	Chmod(path string, mode fs.FileMode) error
}

// osFileFS is FileFS's real implementation, backing every production Output.
type osFileFS struct{}

func (osFileFS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

func (osFileFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (osFileFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	return writeFileAtomic(path, contents, mode)
}

func (osFileFS) Chmod(path string, mode fs.FileMode) error { return os.Chmod(path, mode) }
