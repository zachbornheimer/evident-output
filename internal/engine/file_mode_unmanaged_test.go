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

// TestFileRewriteKeepsModeZeroWhenUnmanaged proves Mode 0 means unmanaged,
// not "create as 0000". An existing file whose mode is 0000 keeps 0000
// when only its contents are rewritten: reading it to compare contents
// must not fail, and the rewrite must not recreate it at 0666.
func TestFileRewriteKeepsModeZeroWhenUnmanaged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "rewrite-zero", FileSpec{Path: path, Contents: []byte("new\n")}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := permOf(t, path); got != 0 {
		t.Fatalf("mode after unmanaged rewrite = %#o, want 0000", got)
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

// TestFileRewriteKeepsSetuidWhenUnmanaged proves an unmanaged rewrite
// keeps the special mode bits too: the existing mode was read through
// Perm(), which drops setuid, setgid, and sticky.
func TestFileRewriteKeepsSetuidWhenUnmanaged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755|fs.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if statOrFatal(t, path).Mode()&fs.ModeSetuid == 0 {
		t.Skip("this filesystem does not keep setuid on a regular file")
	}
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "rewrite", FileSpec{Path: path, Contents: []byte("new\n")}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := statOrFatal(t, path).Mode(); got&fs.ModeSetuid == 0 || got.Perm() != 0o755 {
		t.Fatalf("mode after unmanaged rewrite = %v, want -rwsr-xr-x", got)
	}
}

// v10FileFS is a consumer FileFS written to the v1.0 WriteAtomic
// contract: mode is a real permission it hands straight to os.WriteFile.
// It implements exactly the four FileFS methods and embeds nothing, so it
// cannot inherit osFileFS's own umask-aware create and every write really
// goes through its WriteAtomic.
type v10FileFS struct{}

func (v10FileFS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

func (v10FileFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (v10FileFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	return os.WriteFile(path, contents, mode)
}

func (v10FileFS) Chmod(path string, mode fs.FileMode) error { return os.Chmod(path, mode) }

// lookalikeCreatorFileFS is a consumer FileFS that happens to declare a
// CreateOrdinary method. Only the real filesystem may create a file outside
// WriteAtomic, so the lookalike's method must never be called.
type lookalikeCreatorFileFS struct {
	v10FileFS
	ordinaryCalls, atomicCalls *int
}

func (f lookalikeCreatorFileFS) CreateOrdinary(path string, contents []byte) error {
	*f.ordinaryCalls++
	return os.WriteFile(path, contents, 0o666)
}

func (f lookalikeCreatorFileFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	*f.atomicCalls++
	return f.v10FileFS.WriteAtomic(path, contents, mode)
}

// TestFileCreateIgnoresALookalikeCreateOrdinaryMethod proves a consumer
// FileFS cannot skip WriteAtomic by declaring a method named CreateOrdinary.
func TestFileCreateIgnoresALookalikeCreateOrdinaryMethod(t *testing.T) {
	var ordinaryCalls, atomicCalls int
	fsys := lookalikeCreatorFileFS{ordinaryCalls: &ordinaryCalls, atomicCalls: &atomicCalls}
	path := filepath.Join(t.TempDir(), "new.txt")
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), FileFS: fsys})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "create", FileSpec{Path: path, Contents: []byte("new\n")}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if ordinaryCalls != 0 || atomicCalls != 1 {
		t.Fatalf("CreateOrdinary calls = %d, WriteAtomic calls = %d; want 0 and 1", ordinaryCalls, atomicCalls)
	}
}

// TestFileCreateKeepsV10ModeContractForInjectedFileFS proves an injected
// FileFS still receives a real permission (0666, which the umask masks)
// for an unmanaged create, as in v1.0, never a mode 0 that would leave
// the new file ----------.
func TestFileCreateKeepsV10ModeContractForInjectedFileFS(t *testing.T) {
	if _, creates := any(v10FileFS{}).(ordinaryCreator); creates {
		t.Fatal("v10FileFS creates files itself, so this test would never reach its WriteAtomic")
	}
	withUmask(t, 0o022)
	path := filepath.Join(t.TempDir(), "new.txt")
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), FileFS: v10FileFS{}})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "create", FileSpec{Path: path, Contents: []byte("new\n")}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := permOf(t, path); got != 0o644 {
		t.Fatalf("mode after unmanaged create through a v1.0 FileFS = %v, want -rw-r--r--", got)
	}
}
