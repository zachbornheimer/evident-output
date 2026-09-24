package testkit

import (
	"io/fs"
	"os"
	"sync"

	evo "github.com/zachbornheimer/evident-output"
)

// FileFS is a deterministic evo.FileFS fake: Lstat/ReadFile/WriteAtomic
// delegate to the real filesystem so a test can still exercise evo.File's
// ordinary reconcile logic against a real temp directory, while Chmod can
// be scripted to fail for one path — the one operation a test cannot force
// to fail portably by manipulating a real file's permissions alone.
type FileFS struct {
	mu         sync.Mutex
	chmodFails map[string]error
}

// NewFileFS returns an empty FileFS — script a path's Chmod failure with
// FailChmod before the test's Task runs.
func NewFileFS() *FileFS {
	return &FileFS{chmodFails: map[string]error{}}
}

// FailChmod makes every Chmod call for path return err instead of touching
// the real file.
func (f *FileFS) FailChmod(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chmodFails[path] = err
}

// Lstat implements evo.FileFS.
func (f *FileFS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

// ReadFile implements evo.FileFS.
func (f *FileFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// WriteAtomic implements evo.FileFS by writing directly — this fake exists
// to script Chmod outcomes, not to prove atomic-replace semantics (already
// covered by evo.File's own unit tests).
func (f *FileFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	return os.WriteFile(path, contents, mode)
}

// Chmod implements evo.FileFS, returning the canned failure scripted via
// FailChmod for path — never a real syscall for that path — instead of
// calling os.Chmod.
func (f *FileFS) Chmod(path string, mode fs.FileMode) error {
	if scriptedFailure, ok := f.scriptedChmodFailure(path); ok {
		return scriptedFailure
	}
	return os.Chmod(path, mode)
}

// scriptedChmodFailure returns the canned error FailChmod registered for
// path, if any.
func (f *FileFS) scriptedChmodFailure(path string) (error, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	err, scripted := f.chmodFails[path]
	return err, scripted
}

var _ evo.FileFS = (*FileFS)(nil)
