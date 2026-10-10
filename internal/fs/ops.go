package fs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// File is an open file held by a caller that fills it: write, flush, set
// permissions, close. Holders never see the operating-system handle behind it.
type File interface {
	io.Writer
	// Sync flushes the file's contents to stable storage.
	Sync() error
	// Chmod sets the file's permission bits.
	Chmod(mode fs.FileMode) error
	// Close closes the file.
	Close() error
	// Name is the path the file was created at.
	Name() string
}

// openedFile converts a freshly opened *os.File and its error into a File. A
// failed open yields a nil File, never a non-nil File wrapping a nil pointer.
func openedFile(f *os.File, err error) (File, error) {
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Stat reports path's metadata, following a final symlink.
func Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

// Lstat reports path's own metadata without following a final symlink.
func Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

// ReadFile returns path's full contents.
func ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// ReadDir returns dir's entries sorted by name.
func ReadDir(dir string) ([]fs.DirEntry, error) { return os.ReadDir(dir) }

// Chmod sets path's permission bits.
func Chmod(path string, mode fs.FileMode) error { return os.Chmod(path, mode) }

// Rename moves oldPath to newPath, replacing a non-directory newPath.
func Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// Remove deletes the file or empty directory at path.
func Remove(path string) error { return os.Remove(path) }

// RemoveAll deletes path and everything beneath it; a missing path is success.
func RemoveAll(path string) error { return os.RemoveAll(path) }

// RemoveTree is RemoveAll that also clears read-only directories, such as a
// Go module cache entry (0555 all the way down). It retries only after a
// plain RemoveAll fails, and never follows symlinks.
func RemoveTree(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	makeTreeTraversable(path)
	return os.RemoveAll(path)
}

// makeTreeTraversable gives the owner full access to every directory under
// (and including) root, so RemoveAll can empty and unlink it. The walk runs
// inside an os.Root opened at root, so a path it visits can never resolve
// through a symlink to somewhere outside the tree.
func makeTreeTraversable(path string) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return
	}
	// A directory with no owner access cannot be opened as a root.
	if err := os.Chmod(path, ownerDirAccess(info.Mode())); err != nil {
		return
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()
	_ = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, _ error) error {
		if d != nil && d.IsDir() {
			if dirInfo, err := root.Lstat(p); err == nil {
				_ = root.Chmod(p, ownerDirAccess(dirInfo.Mode()))
			}
		}
		return nil
	})
}

// ownerDirAccess is mode's permission bits plus owner read, write and
// search, so the owner can list, empty and remove the directory.
func ownerDirAccess(mode fs.FileMode) fs.FileMode {
	return mode.Perm() | fs.FileMode(ownerReadWriteSearch)
}

// ownerReadWriteSearch is the owner's rwx permission bits.
const ownerReadWriteSearch = 0o700

// Mkdir creates one directory.
func Mkdir(path string, mode fs.FileMode) error { return os.Mkdir(path, mode) }

// MkdirAll creates path and any missing parents.
func MkdirAll(path string, mode fs.FileMode) error { return os.MkdirAll(path, mode) }

// MkdirTemp creates a new temporary directory in dir (the system temporary
// directory when dir is empty) named from pattern.
func MkdirTemp(dir, pattern string) (string, error) { return os.MkdirTemp(dir, pattern) }

// CreateTemp creates a new temporary file in dir named from pattern.
func CreateTemp(dir, pattern string) (File, error) { return openedFile(os.CreateTemp(dir, pattern)) }

// CreateExclusive creates a new read-write file at path with mode; it fails
// when path already exists.
func CreateExclusive(path string, mode fs.FileMode) (File, error) {
	return openedFile(os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode))
}

// Getwd returns the process's current working directory.
func Getwd() (string, error) { return os.Getwd() }

// Executable returns the path of the running executable.
func Executable() (string, error) { return os.Executable() }

// UserCacheDir returns the user's cache directory.
func UserCacheDir() (string, error) { return os.UserCacheDir() }

// TempDir returns the system temporary directory.
func TempDir() string { return os.TempDir() }

// Abs returns path as an absolute, cleaned path: a relative path is joined to
// the process's current working directory. It does not resolve symlinks.
func Abs(path string) (string, error) { return filepath.Abs(path) }

// EvalSymlinks returns path with every symlink resolved: its canonical
// spelling.
func EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }
