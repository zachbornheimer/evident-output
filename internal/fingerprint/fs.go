package fingerprint

import (
	"io/fs"
	"os"
	"sort"
)

// FS is the facade every FSPath observation reads through instead of the
// os package directly (facade rule) — swapped for a fake in this package's
// own tests via withFS.
type FS interface {
	// Lstat reports Path's own metadata without following a final symlink.
	// A missing path returns an error satisfying os.IsNotExist.
	Lstat(path string) (fs.FileInfo, error)
	// ReadFile returns a regular file's full contents.
	ReadFile(path string) ([]byte, error)
	// ReadDir returns a directory's entries, any order.
	ReadDir(path string) ([]fs.DirEntry, error)
	// Readlink returns a symlink's raw target text.
	Readlink(path string) (string, error)
}

// osFS is the real filesystem.
type osFS struct{}

func (osFS) Lstat(path string) (fs.FileInfo, error)     { return os.Lstat(path) }
func (osFS) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (osFS) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (osFS) Readlink(path string) (string, error)       { return os.Readlink(path) }

// activeFS is the package-level FS seam. Production code always observes
// the real filesystem; only this package's own white-box tests swap it.
var activeFS FS = osFS{}

// withFS runs fn with the package's FS facade swapped to f, restoring the
// previous facade afterward. Test-only: unexported, used from
// fingerprint_test.go/fspath_test.go in this package.
func withFS(f FS, fn func()) {
	prev := activeFS
	activeFS = f
	defer func() { activeFS = prev }()
	fn()
}

// sortedDirEntryNames returns names, sorted, for deterministic Merkle
// traversal order (spec §11.1: "deterministic ... over sorted relative
// names").
func sortedDirEntryNames(entries []fs.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)
	return names
}
