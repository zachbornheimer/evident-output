//go:build unix

package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// withUmask sets the process umask for one test and restores it after.
// Callers must not run in parallel: umask is process-wide.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	previous := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(previous) })
}

func permOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	return statOrFatal(t, path).Mode().Perm()
}

// TestFileRewriteKeepsExistingModeWhenUnmanaged proves an unmanaged-mode
// (Mode == 0) content rewrite leaves the file's current permissions alone.
func TestFileRewriteKeepsExistingModeWhenUnmanaged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.sh")
	if err := os.WriteFile(path, []byte("old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "rewrite", FileSpec{Path: path, Contents: []byte("new\n")}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := permOf(t, path); got != 0o755 {
		t.Fatalf("mode after unmanaged rewrite = %v, want -rwxr-xr-x", got)
	}
}

// TestFileCreateHonorsUmaskWhenUnmanaged proves an unmanaged-mode create
// uses ordinary creation semantics: 0666 less the umask.
func TestFileCreateHonorsUmaskWhenUnmanaged(t *testing.T) {
	withUmask(t, 0o022)
	path := filepath.Join(t.TempDir(), "new.txt")
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "create", FileSpec{Path: path, Contents: []byte("new\n")}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := permOf(t, path); got != 0o644 {
		t.Fatalf("mode after unmanaged create under umask 022 = %v, want -rw-r--r--", got)
	}
}

// TestPatchKeepsExecutableBit proves a content-only diff committed through
// Patch + Files keeps a script's executable bit.
func TestPatchKeepsExecutableBit(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	path := filepath.Join(dir, "greeting.txt")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runFilesTask(t, out, modifyGreetingDiff, nil); err != nil {
		t.Fatalf("Files: %v", err)
	}
	if got := readOrFatal(t, path); got != "hello\nthere\n" {
		t.Fatalf("contents = %q", got)
	}
	if got := permOf(t, path); got != 0o755 {
		t.Fatalf("mode after content-only Patch = %v, want -rwxr-xr-x", got)
	}
}
