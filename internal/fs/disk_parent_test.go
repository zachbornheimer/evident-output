package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/fs"
)

// TestDiskWriteAtomicCreatesMissingParents pins that the real filesystem
// creates every missing directory above the file, so a file's desired state
// never fails because its directory is not there yet.
func TestDiskWriteAtomicCreatesMissingParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out", "deep", "a.html")
	if err := fs.System().WriteAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteAtomic(%q): %v", path, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "x" {
		t.Fatalf("ReadFile(%q) = %q, %v; want x", path, got, err)
	}
}
