package fs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// When the root's own chmod fails (a filesystem that refuses chmod) but the
// root is already openable, the read-only directories beneath it must still be
// opened up so the tree can be removed. The earlier WalkDir-based walk ignored
// the root's chmod failure and carried on; makeTreeTraversable must too.
func TestMakeTreeTraversableOpensChildrenWhenRootChmodFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes read-only directories without chmod")
	}
	tree := filepath.Join(t.TempDir(), "tree")
	nested := filepath.Join(tree, "a")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(nested, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(nested, 0o700) })

	refuseChmod := func(string, fs.FileMode) error { return errors.New("chmod refused") }
	makeTreeTraversable(tree, refuseChmod)

	if err := os.RemoveAll(tree); err != nil {
		t.Fatalf("RemoveAll after makeTreeTraversable with a refused root chmod: %v", err)
	}
}
