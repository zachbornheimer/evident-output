package fingerprint

import (
	"io/fs"
	"sort"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// FS is the facade every FSPath observation reads through instead of the
// os package directly (facade rule) — swapped for a fake in this package's
// own tests via withFS.
type FS = sysfs.Inspector

// activeFS is the package-level FS seam. Production code always observes
// the real filesystem; only this package's own white-box tests swap it.
var activeFS FS = sysfs.System()

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
