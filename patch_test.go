package evo

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
)

// patchWorkspaceCeiling lets a test hold one writer mid-callback while a second
// Task runs, regardless of GOMAXPROCS.
const patchWorkspaceCeiling = 2

// patchWorkspace is a temp workspace and an Output whose relative paths
// resolve inside it.
func patchWorkspace(t *testing.T, files map[string]string) (string, *Run) {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := initRun(Config{MaxConcurrency: patchWorkspaceCeiling, State: State{Dir: t.TempDir()}})
	t.Cleanup(func() { _ = out.close() })
	out.changes.PinWorkspace(dir)
	return dir, out
}

// runPatchTask calls Patch from one Task's Define callback.
func runPatchTask(t *testing.T, out *Run, diff string) (change.FileSet, error) {
	t.Helper()
	var set change.FileSet
	var patchErr error
	task := out.Task("derive patch")
	task.Define(func(ctx context.Context) error {
		set, patchErr = derivePatch(ctx, []byte(diff))
		return patchErr
	})
	_ = task.wait()
	return set, patchErr
}

// treeSnapshot records every entry under dir: its mode, mtime, and bytes.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		entry := info.Mode().String() + " " + info.ModTime().String()
		if d.Type().IsRegular() {
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			entry += " " + string(contents)
		}
		snap[path] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// assertTreeUnchanged fails naming every entry that differs between two
// snapshots.
func assertTreeUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	if maps.Equal(before, after) {
		return
	}
	for path := range maps.Keys(before) {
		if before[path] != after[path] {
			t.Errorf("%s: before %q, after %q", path, before[path], after[path])
		}
	}
	for path := range maps.Keys(after) {
		if _, existed := before[path]; !existed {
			t.Errorf("%s: created by Patch", path)
		}
	}
}

const modifyGreetingDiff = `diff --git a/greeting.txt b/greeting.txt
--- a/greeting.txt
+++ b/greeting.txt
@@ -1,2 +1,2 @@
 hello
-world
+there
`

const createDocDiff = `diff --git a/docs/new.md b/docs/new.md
new file mode 100644
--- /dev/null
+++ b/docs/new.md
@@ -0,0 +1 @@
+# New
`

const twoFileDiff = modifyGreetingDiff + createDocDiff

// docsWorkspace holds greeting.txt and the docs directory createDocDiff
// creates a file in.
var docsWorkspace = map[string]string{"greeting.txt": "hello\nworld\n", "docs/README.md": "docs\n"}

func TestPatchPerformsNoWorkspaceMutation(t *testing.T) {
	dir, out := patchWorkspace(t, docsWorkspace)
	before := treeSnapshot(t, dir)
	if _, err := runPatchTask(t, out, twoFileDiff); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, dir))
}

func TestPatchUnsupportedFormsFailExplicitly(t *testing.T) {
	cases := map[string]struct {
		diff string
		want error
	}{
		"delete": {"diff --git a/greeting.txt b/greeting.txt\ndeleted file mode 100644\n", change.ErrPatchDeleteUnsupported},
		"rename": {"diff --git a/greeting.txt b/g.txt\nrename from greeting.txt\nrename to g.txt\n", change.ErrPatchRenameUnsupported},
		"binary": {"diff --git a/greeting.txt b/greeting.txt\nBinary files a/greeting.txt and b/greeting.txt differ\n", change.ErrPatchBinaryUnsupported},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
			set, err := runPatchTask(t, out, tc.diff)
			if !errors.Is(err, tc.want) || !errors.Is(err, change.ErrPatchUnsupported) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(set, change.FileSet{}) {
				t.Fatal("a failed Patch returned desired files")
			}
		})
	}
}

func TestPatchSourceMismatchFails(t *testing.T) {
	cases := map[string]string{
		"context differs":      modifyGreetingDiff,
		"create over existing": "--- /dev/null\n+++ b/greeting.txt\n@@ -0,0 +1 @@\n+x\n",
		"modify missing":       "--- a/absent.txt\n+++ b/absent.txt\n@@ -1 +1 @@\n-a\n+b\n",
	}
	for name, diff := range cases {
		t.Run(name, func(t *testing.T) {
			_, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nplanet\n"})
			if _, err := runPatchTask(t, out, diff); !errors.Is(err, ErrPatchDoesNotApply) {
				t.Fatalf("err = %v, want ErrPatchDoesNotApply", err)
			}
		})
	}
}

func TestPatchRejectsSymlinkSource(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"real.txt": "hello\nworld\n"})
	if err := os.Symlink(filepath.Join(dir, "real.txt"), filepath.Join(dir, "greeting.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := runPatchTask(t, out, modifyGreetingDiff); !errors.Is(err, change.ErrFilePathIsSymlink) {
		t.Fatalf("err = %v, want ErrFilePathIsSymlink", err)
	}
}

func TestPatchRequiresTaskContext(t *testing.T) {
	if err := Patch(context.Background(), []byte(twoFileDiff)); !errors.Is(err, graph.ErrNoTaskContext) {
		t.Fatalf("err = %v, want ErrNoTaskContext", err)
	}
}

func TestPatchCommitsTheDerivedFilesAndReturnsOnlyAnError(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	var patchErr error
	task := out.Task("apply patch")
	task.Define(func(ctx context.Context) error {
		patchErr = Patch(ctx, []byte(modifyGreetingDiff))
		return patchErr
	})
	_ = task.wait()
	if patchErr != nil {
		t.Fatalf("Patch: %v", patchErr)
	}
	if got := readOrFatal(t, filepath.Join(dir, "greeting.txt")); got == "hello\nworld\n" {
		t.Fatalf("greeting.txt unchanged after Patch: %q", got)
	}
}
