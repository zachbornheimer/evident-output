package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

// Every direct os call evo.File makes lives in this file, behind FileFS
// and getwd, so file.go and file_reconcile.go hold only reconciliation
// logic.

// getwd is the facade File's relative-path resolution reads the process's
// current working directory through, instead of os.Getwd directly (facade
// rule).
var getwd = os.Getwd

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
	// concurrent reader could observe. mode 0 means ordinary creation
	// semantics: 0666 less the process umask.
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
