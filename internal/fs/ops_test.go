package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/fs"
)

// Abs joins a relative path to the working directory and cleans an absolute one.
func TestAbsResolvesAgainstTheWorkingDirectory(t *testing.T) {
	wd, err := fs.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs.Abs(filepath.Join("a", "..", "b"))
	if err != nil {
		t.Fatalf("Abs relative: %v", err)
	}
	if want := filepath.Join(wd, "b"); got != want {
		t.Fatalf("Abs relative = %q, want %q", got, want)
	}
	got, err = fs.Abs(filepath.Join(wd, "x", "..", "y"))
	if err != nil {
		t.Fatalf("Abs absolute: %v", err)
	}
	if want := filepath.Join(wd, "y"); got != want {
		t.Fatalf("Abs absolute = %q, want %q", got, want)
	}
}

// A read-only tree (a Go module cache entry is 0555 all the way down) is
// removed, and a symlink inside it is unlinked without touching its target.
func TestRemoveTreeClearsReadOnlyDirectories(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(base, "tree")
	nested := filepath.Join(tree, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(tree, "link")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{nested, filepath.Join(tree, "a"), tree} {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
	}

	if err := fs.RemoveTree(tree); err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if _, err := os.Lstat(tree); !os.IsNotExist(err) {
		t.Fatalf("tree still exists: %v", err)
	}
	if info, err := os.Lstat(outside); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("symlink target was touched: %v, %v", info, err)
	}
}
