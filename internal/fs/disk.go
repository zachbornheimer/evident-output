// Package fs is the one place Evo touches the filesystem: reading and
// writing files, taking advisory locks, swapping directory entries and
// resolving canonical paths. Every other internal package takes an FS (or
// calls a function here) and imports no os.
package fs

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FS is the facade evo.File performs every filesystem inspection and
// mutation through, instead of the os package directly: Disk backs every
// production Output; a test injects a fake to script a filesystem outcome
// (e.g. a chmod failure) deterministically, without depending on a real
// disk's permission behavior.
type FS interface {
	// Lstat inspects path without following a symlink — File's own
	// existence/type check (spec §8.2).
	Lstat(path string) (fs.FileInfo, error)
	// ReadFile reads path's current contents for File's content-diff check.
	ReadFile(path string) ([]byte, error)
	// WriteAtomic replaces path's contents with contents at mode, atomically
	// (spec §8.2's "write replacement contents") — never a partial write a
	// concurrent reader could observe. mode is always a real permission:
	// the managed Mode, an existing file's own mode, or 0666 for a new
	// file whose mode is unmanaged.
	WriteAtomic(path string, contents []byte, mode fs.FileMode) error
	// Chmod sets path's permission bits.
	Chmod(path string, mode fs.FileMode) error
}

// Inspector is the read-only view fingerprinting observes a path through.
type Inspector interface {
	// Lstat reports path's own metadata without following a final symlink.
	// A missing path returns an error satisfying errors.Is(err, fs.ErrNotExist).
	Lstat(path string) (fs.FileInfo, error)
	// ReadFile returns a regular file's full contents.
	ReadFile(path string) ([]byte, error)
	// ReadDir returns a directory's entries, any order.
	ReadDir(path string) ([]fs.DirEntry, error)
	// Readlink returns a symlink's raw target text.
	Readlink(path string) (string, error)
}

// ordinaryCreator is an FS that can create a file under the process umask
// itself. Its method is unexported, so only Disk is one: no consumer FS can
// opt out of WriteAtomic by declaring a method of the same name.
type ordinaryCreator interface {
	createOrdinary(path string, contents []byte) error
}

// CreateOrdinary creates path with ordinary creation semantics (0666 less the
// process umask) when fsys is the real filesystem. Any other FS receives
// WriteAtomic(path, contents, UnmanagedCreateMode), as in v1.0, which
// os.WriteFile and os.OpenFile mask by the umask.
func CreateOrdinary(fsys FS, path string, contents []byte) error {
	if creator, ok := fsys.(ordinaryCreator); ok {
		return creator.createOrdinary(path, contents)
	}
	return fsys.WriteAtomic(path, contents, UnmanagedCreateMode)
}

// UnmanagedCreateMode is the permission a new file whose mode is unmanaged
// is created with, before the umask.
const UnmanagedCreateMode fs.FileMode = 0o666

// tempNameAttempts bounds createTempFile's search for an unused name.
const tempNameAttempts = 10000

// Disk is the real filesystem. It is the FS and the Inspector every
// production Output uses.
type Disk struct{}

// System returns the real filesystem.
func System() Disk { return Disk{} }

// Lstat implements FS and Inspector.
func (Disk) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

// ReadFile implements FS and Inspector.
func (Disk) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// ReadDir implements Inspector.
func (Disk) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }

// Readlink implements Inspector.
func (Disk) Readlink(path string) (string, error) { return os.Readlink(path) }

// WriteAtomic implements FS.
func (Disk) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	return writeFileAtomic(path, contents, exactPermission(mode))
}

// Chmod implements FS.
func (Disk) Chmod(path string, mode fs.FileMode) error { return os.Chmod(path, mode) }

// createOrdinary creates path with ordinary creation semantics: 0666 less
// the process umask.
func (Disk) createOrdinary(path string, contents []byte) error {
	return writeFileAtomic(path, contents, ordinaryPermission)
}

// permission is the mode writeFileAtomic leaves: exact, or ordinary
// creation semantics under the umask.
type permission struct {
	mode  fs.FileMode
	exact bool
}

// ordinaryPermission is 0666 masked by the umask.
var ordinaryPermission = permission{mode: UnmanagedCreateMode}

// exactPermission is exactly mode, whatever the umask.
func exactPermission(mode fs.FileMode) permission { return permission{mode: mode, exact: true} }

// tempPermission is the mode the temp file opens with: owner-only until
// an exact chmod, otherwise the ordinary mode the umask then masks.
func (p permission) tempPermission() fs.FileMode {
	if p.exact {
		return 0o600
	}
	return p.mode
}

// writeFileAtomic writes contents to a temp file beside path and renames it
// into place — the same atomic-replace contract the manifest store's
// writeAtomic uses — so a reader never observes a partially written file.
// The file ends with perm (spec §8.1: unmanaged mode is never claimed).
func writeFileAtomic(path string, contents []byte, perm permission) error {
	dir := filepath.Dir(path)
	tmp, err := createTempFile(dir, perm.tempPermission())
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := writeAndClose(tmp, contents); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fill temp file %q: %w", tmpPath, err)
	}
	if perm.exact {
		if err := os.Chmod(tmpPath, perm.mode); err != nil {
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
		name := filepath.Join(dir, ".evo-file-"+rand.Text()+".tmp")
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", name, err)
		}
		return f, nil
	}
	return nil, fmt.Errorf("no unused temp file name after %d attempts", tempNameAttempts)
}

// writeAndClose writes contents to f, fsyncs it, and closes it.
func writeAndClose(f *os.File, contents []byte) error {
	if _, err := f.Write(contents); err != nil {
		_ = f.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}
