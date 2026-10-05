package checksum

import (
	"fmt"
	"io"
	"io/fs"
	"os"
)

// Source is the filesystem the engine observes. Every byte and every
// directory listing the engine hashes comes through it, which is what
// lets a test (or a future snapshot) prove a digest's provenance.
type Source interface {
	// Lstat inspects path without following a symlink.
	Lstat(path string) (fs.FileInfo, error)
	// Open streams path's bytes. The engine only opens paths Lstat
	// reported as regular files.
	Open(path string) (io.ReadCloser, error)
	// ReadDir lists a directory's entries in any order.
	ReadDir(path string) ([]fs.DirEntry, error)
	// Readlink returns a symlink's target text, unresolved.
	Readlink(path string) (string, error)
}

// OS is the real filesystem. Its Open never blocks on a FIFO swapped in
// after Lstat and never follows a symlink swapped in after Lstat.
type OS struct{}

// Lstat implements Source.
func (OS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

// Open implements Source.
func (OS) Open(path string) (io.ReadCloser, error) { return openRegular(path) }

// ReadDir implements Source.
func (OS) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }

// Readlink implements Source.
func (OS) Readlink(path string) (string, error) { return os.Readlink(path) }

// openedNonRegular is the error from opening a path whose type changed to a
// non-regular file between Lstat and open.
func openedNonRegular(path string, mode fs.FileMode) error {
	return fmt.Errorf("%w: %s is %v, not a regular file", ErrNotRegular, path, mode.Type())
}
