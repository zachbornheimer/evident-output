package tree_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Modes of a sealed store tree, and of a writable clone of it.
const (
	sealedDirMode      fs.FileMode = 0o555
	sealedFileMode     fs.FileMode = 0o444
	sealedExecMode     fs.FileMode = 0o555
	writableDirMode    fs.FileMode = 0o755
	writableFileMode   fs.FileMode = 0o644
	writableExecMode   fs.FileMode = 0o755
	pruneExecutableRel             = "bin/run"
)

// sealedCloneSource is cloneSource sealed the way a content store seals
// it: nothing writable, the executable still executable.
func sealedCloneSource(t *testing.T) string {
	t.Helper()
	src := cloneSource(t)
	pruneChmodTree(t, src, sealedDirMode, sealedFileMode, sealedExecMode)
	t.Cleanup(func() { pruneChmodTree(t, src, writableDirMode, writableFileMode, writableExecMode) })
	return src
}

// pruneChmodTree sets every directory and regular file under root (root
// included) to dir, file, or exec (for files with an execute bit).
// Directories are opened before their own mode is applied.
func pruneChmodTree(t *testing.T, root string, dir, file, exec fs.FileMode) {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			dirs = append(dirs, path)
			return os.Chmod(path, writableDirMode)
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&0o111 != 0 {
				return os.Chmod(path, exec)
			}
			return os.Chmod(path, file)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir0 := range slices.Backward(dirs) {
		if err := os.Chmod(dir0, dir); err != nil {
			t.Fatal(err)
		}
	}
}

// pruneModes is every entry under root (root excluded, symlinks skipped)
// as slash path -> permission bits.
func pruneModes(t *testing.T, root string) map[string]fs.FileMode {
	t.Helper()
	modes := map[string]fs.FileMode{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root || entry.Type()&fs.ModeSymlink != 0 {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		modes[filepath.ToSlash(rel)] = info.Mode().Perm()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return modes
}

func writeWritableClone(t *testing.T, src, dst string) {
	t.Helper()
	tree := evo.Tree{Path: dst, Content: evo.Clone{From: evo.Tree{Path: src}, Writable: true}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write(Clone{Writable}) = %v", err)
	}
}

func TestPrune_WritableCloneOfASealedTreeIsOwnerWritableWithTheSameDigest(t *testing.T) {
	src := sealedCloneSource(t)
	sealed := pruneModes(t, src)
	dst := filepath.Join(t.TempDir(), "pkg")
	writeWritableClone(t, src, dst)

	for rel, mode := range pruneModes(t, dst) {
		want := writableFileMode
		if info, err := os.Lstat(filepath.Join(dst, rel)); err == nil && info.IsDir() {
			want = writableDirMode
		} else if rel == pruneExecutableRel {
			want = writableExecMode
		}
		if mode != want {
			t.Errorf("clone %s mode = %v, want %v", rel, mode, want)
		}
	}
	if got, want := pruneChecksum(t, dst), pruneChecksum(t, src); got != want {
		t.Fatalf("writable clone digests to %s, source to %s; only the exec bit may count", got, want)
	}
	for rel, mode := range pruneModes(t, src) {
		if mode != sealed[rel] {
			t.Errorf("cloning changed the sealed source %s mode %v -> %v", rel, sealed[rel], mode)
		}
	}
	if err := os.RemoveAll(dst); err != nil {
		t.Fatalf("a writable clone could not be removed: %v", err)
	}
}

func TestPrune_DefaultCloneOfASealedTreeKeepsTheSourceModes(t *testing.T) {
	src := sealedCloneSource(t)
	dst := filepath.Join(t.TempDir(), "pkg")
	tree := evo.Tree{Path: dst, Content: evo.Clone{From: evo.Tree{Path: src}}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write(Clone) = %v", err)
	}
	t.Cleanup(func() { pruneChmodTree(t, dst, writableDirMode, writableFileMode, writableExecMode) })
	if got := pruneModes(t, dst)["lib/a.js"]; got != sealedFileMode {
		t.Fatalf("default clone lib/a.js mode = %v, want the source's %v", got, sealedFileMode)
	}
}
