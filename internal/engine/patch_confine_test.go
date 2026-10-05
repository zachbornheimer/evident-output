package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linkedWorkspace is a patch workspace holding "link", a symlink to a
// directory outside it.
func linkedWorkspace(t *testing.T) (dir, outside string, out *Output) {
	t.Helper()
	dir, out = patchWorkspace(t, nil)
	outside = t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	return dir, outside, out
}

const createThroughLinkDiff = "--- /dev/null\n+++ b/link/evil.txt\n@@ -0,0 +1 @@\n+pwned\n"

// TestPatchRefusesPathBeyondSymlinkedDirectory proves a diff path whose
// parent is a symlinked directory is refused at Patch, before anything is
// read or written through the link (git apply: "beyond a symbolic link").
func TestPatchRefusesPathBeyondSymlinkedDirectory(t *testing.T) {
	_, outside, out := linkedWorkspace(t)
	err := runFilesTask(t, out, createThroughLinkDiff, nil)
	if !errors.Is(err, ErrPatchUnsupported) {
		t.Fatalf("err = %v, want ErrPatchUnsupported", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "evil.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Patch+Files wrote outside the workspace through link (stat err %v)", statErr)
	}
}

// TestFilesRevalidatesParentsAtCommit proves a parent directory swapped
// for a symlink after Patch derived the file is refused at commit.
func TestFilesRevalidatesParentsAtCommit(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"sub/keep.txt": "x\n"})
	outside := t.TempDir()
	diff := "--- /dev/null\n+++ b/sub/evil.txt\n@@ -0,0 +1 @@\n+pwned\n"
	err := runFilesTask(t, out, diff, func() {
		if renameErr := os.Rename(filepath.Join(dir, "sub"), filepath.Join(dir, "sub.moved")); renameErr != nil {
			t.Fatal(renameErr)
		}
		if linkErr := os.Symlink(outside, filepath.Join(dir, "sub")); linkErr != nil {
			t.Fatal(linkErr)
		}
	})
	if !errors.Is(err, ErrPatchUnsupported) {
		t.Fatalf("err = %v, want ErrPatchUnsupported", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "evil.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Files wrote outside the workspace through a swapped-in link (stat err %v)", statErr)
	}
}

// TestPatchRefusesCreationInMissingDirectory proves a creation whose
// parent directory does not exist fails at Patch with a named error, so
// Files never commits part of the set or surfaces a temp-file path.
func TestPatchRefusesCreationInMissingDirectory(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	diff := modifyGreetingDiff + "diff --git a/d/e/new.sh b/d/e/new.sh\nnew file mode 100755\n--- /dev/null\n+++ b/d/e/new.sh\n@@ -0,0 +1 @@\n+echo hi\n"
	err := runFilesTask(t, out, diff, nil)
	if !errors.Is(err, ErrPatchDoesNotApply) {
		t.Fatalf("err = %v, want ErrPatchDoesNotApply", err)
	}
	if strings.Contains(err.Error(), ".evo-file-") {
		t.Fatalf("err names evo's temp file: %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join("d", "e")) {
		t.Fatalf("err does not name the missing directory: %v", err)
	}
	if got := readOrFatal(t, filepath.Join(dir, "greeting.txt")); got != "hello\nworld\n" {
		t.Fatalf("greeting.txt = %q, want untouched: the set must fail before any commit", got)
	}
}
