// Package patch_test is the ZYS-1382 execution contract for evo.Patch: one
// direct apply surface for unified diffs that commits every touched path
// through the File machinery, or nothing. Names and open-syntax choices:
// docs/zys-1382/contract-decisions.md.
package patch_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// workspace makes a temp dir the working directory (diff paths resolve
// against it), plants files, and returns it.
func workspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	plant(t, dir, files)
	t.Chdir(dir)
	return dir
}

// plant writes files (slash-separated relative path -> content) under root.
func plant(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// apply runs evo.Patch inside a Task, optionally with between executed
// after the Task starts and before Patch is called.
func apply(t *testing.T, cfg evo.Config, diff string, between func()) error {
	t.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = t.TempDir()
	}
	out := evo.Init(cfg)
	var got error
	task := out.Task("patch").Define(func(ctx context.Context) error {
		if between != nil {
			between()
		}
		got = evo.Patch(ctx, []byte(diff))
		return got
	})
	_ = task.Wait()
	_ = out.Finish()
	return got
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

const modifyDiff = `--- a/notes.txt
+++ b/notes.txt
@@ -1 +1 @@
-draft
+final
`

const multiFileDiff = `diff --git a/notes.txt b/notes.txt
--- a/notes.txt
+++ b/notes.txt
@@ -1 +1 @@
-draft
+final
diff --git a/docs/README.md b/docs/README.md
--- a/docs/README.md
+++ b/docs/README.md
@@ -1 +1,2 @@
 docs
+more
diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+created
`

const createDiff = `diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+first
+second
`

const deleteDiff = `diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
`

const renameDiff = `diff --git a/old.txt b/renamed.txt
similarity index 100%
rename from old.txt
rename to renamed.txt
`

const renameWithEditDiff = `diff --git a/old.txt b/renamed.txt
similarity index 60%
rename from old.txt
rename to renamed.txt
--- a/old.txt
+++ b/renamed.txt
@@ -1 +1 @@
-before
+after
`

const modeDiff = `diff --git a/run.sh b/run.sh
old mode 100644
new mode 100755
`

const modeWithEditDiff = `diff --git a/run.sh b/run.sh
old mode 100644
new mode 100755
--- a/run.sh
+++ b/run.sh
@@ -1 +1 @@
-echo one
+echo two
`

func TestPatchIsOneDirectApplySurface(t *testing.T) {
	// Compiling this slice literal pins evo.Patch to func(ctx, diff []byte) error.
	surfaces := []func(ctx context.Context, diff []byte) error{evo.Patch}
	if len(surfaces) != 1 {
		t.Fatalf("evo.Patch must be func(ctx, diff []byte) error")
	}
}

func TestPatchAppliesAUnifiedDiff(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
	if err := apply(t, evo.Config{}, modifyDiff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "final\n" {
		t.Fatalf("notes.txt = %q, want %q", got, "final\n")
	}
}

func TestPatchAppliesEveryFileOfAMultiFileDiff(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n", "docs/README.md": "docs\n"})
	if err := apply(t, evo.Config{}, multiFileDiff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	want := map[string]string{"notes.txt": "final\n", "docs/README.md": "docs\nmore\n", "new.txt": "created\n"}
	for rel, body := range want {
		if got := read(t, filepath.Join(dir, rel)); got != body {
			t.Fatalf("%s = %q, want %q", rel, got, body)
		}
	}
}

func TestPatchStandardForms(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		diff   string
		assert func(t *testing.T, dir string)
	}{
		{"create", map[string]string{}, createDiff, func(t *testing.T, dir string) {
			if got := read(t, filepath.Join(dir, "new.txt")); got != "first\nsecond\n" {
				t.Fatalf("new.txt = %q", got)
			}
		}},
		{"delete", map[string]string{"gone.txt": "bye\n", "kept.txt": "stay\n"}, deleteDiff, func(t *testing.T, dir string) {
			if exists(filepath.Join(dir, "gone.txt")) {
				t.Fatalf("gone.txt still exists")
			}
			if !exists(filepath.Join(dir, "kept.txt")) {
				t.Fatalf("kept.txt was removed")
			}
		}},
		{"rename", map[string]string{"old.txt": "same\n"}, renameDiff, func(t *testing.T, dir string) {
			if exists(filepath.Join(dir, "old.txt")) {
				t.Fatalf("old.txt still exists after rename")
			}
			if got := read(t, filepath.Join(dir, "renamed.txt")); got != "same\n" {
				t.Fatalf("renamed.txt = %q", got)
			}
		}},
		{"rename with edit", map[string]string{"old.txt": "before\n"}, renameWithEditDiff, func(t *testing.T, dir string) {
			if exists(filepath.Join(dir, "old.txt")) {
				t.Fatalf("old.txt still exists after rename")
			}
			if got := read(t, filepath.Join(dir, "renamed.txt")); got != "after\n" {
				t.Fatalf("renamed.txt = %q", got)
			}
		}},
		{"mode change", map[string]string{"run.sh": "echo one\n"}, modeDiff, func(t *testing.T, dir string) {
			info, err := os.Stat(filepath.Join(dir, "run.sh"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("run.sh mode = %v, want executable", info.Mode().Perm())
			}
			if got := read(t, filepath.Join(dir, "run.sh")); got != "echo one\n" {
				t.Fatalf("a mode-only diff changed content: %q", got)
			}
		}},
		{"mode change with edit", map[string]string{"run.sh": "echo one\n"}, modeWithEditDiff, func(t *testing.T, dir string) {
			info, err := os.Stat(filepath.Join(dir, "run.sh"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("run.sh mode = %v, want executable", info.Mode().Perm())
			}
			if got := read(t, filepath.Join(dir, "run.sh")); got != "echo two\n" {
				t.Fatalf("run.sh = %q", got)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := workspace(t, tc.files)
			if err := apply(t, evo.Config{}, tc.diff, nil); err != nil {
				t.Fatalf("Patch: %v", err)
			}
			tc.assert(t, dir)
		})
	}
}

func TestPatchCreatesMissingParentDirectories(t *testing.T) {
	const diff = `diff --git a/deep/er/new.txt b/deep/er/new.txt
new file mode 100644
--- /dev/null
+++ b/deep/er/new.txt
@@ -0,0 +1 @@
+hi
`
	dir := workspace(t, map[string]string{})
	if err := apply(t, evo.Config{}, diff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "deep", "er", "new.txt")); got != "hi\n" {
		t.Fatalf("deep/er/new.txt = %q", got)
	}
}

func TestPatchAlreadyAppliedIsSatisfied(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "final\n"})
	if err := apply(t, evo.Config{}, modifyDiff, nil); err != nil {
		t.Fatalf("Patch of an already-applied diff = %v, want nil", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "final\n" {
		t.Fatalf("notes.txt = %q", got)
	}
}

func TestPatchValidatesApplicabilityBeforeCommittingAnything(t *testing.T) {
	// The first file applies cleanly; the second does not. Nothing changes.
	const diff = `diff --git a/notes.txt b/notes.txt
--- a/notes.txt
+++ b/notes.txt
@@ -1 +1 @@
-draft
+final
diff --git a/docs/README.md b/docs/README.md
--- a/docs/README.md
+++ b/docs/README.md
@@ -1 +1 @@
-not what is there
+more
`
	dir := workspace(t, map[string]string{"notes.txt": "draft\n", "docs/README.md": "docs\n"})
	if err := apply(t, evo.Config{}, diff, nil); !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("Patch = %v, want ErrPatchDoesNotApply", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "draft\n" {
		t.Fatalf("notes.txt = %q; a diff that fails later still committed an earlier file", got)
	}
	if got := read(t, filepath.Join(dir, "docs", "README.md")); got != "docs\n" {
		t.Fatalf("README.md = %q, want untouched", got)
	}
}

func TestPatchRejectsFormsItCannotApply(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		diff  string
		want  error
	}{
		{"malformed", map[string]string{}, "this is not a diff\n", evo.ErrPatchMalformed},
		{"empty", map[string]string{}, "", evo.ErrPatchMalformed},
		{"hunk does not match", map[string]string{"notes.txt": "other\n"}, modifyDiff, evo.ErrPatchDoesNotApply},
		{"modify a missing file", map[string]string{}, modifyDiff, evo.ErrPatchDoesNotApply},
		{"create an existing file", map[string]string{"new.txt": "already\n"}, createDiff, evo.ErrPatchDoesNotApply},
		{"delete a missing file", map[string]string{}, deleteDiff, evo.ErrPatchDoesNotApply},
		{"delete a file with other content", map[string]string{"gone.txt": "not bye\n"}, deleteDiff, evo.ErrPatchDoesNotApply},
		{"rename a missing file", map[string]string{}, renameDiff, evo.ErrPatchDoesNotApply},
		{"rename onto an existing file", map[string]string{"old.txt": "same\n", "renamed.txt": "occupied\n"}, renameDiff, evo.ErrPatchDoesNotApply},
		{"rename with an edit that does not match", map[string]string{"old.txt": "other\n"}, renameWithEditDiff, evo.ErrPatchDoesNotApply},
		{"mode change of a missing file", map[string]string{}, modeDiff, evo.ErrPatchDoesNotApply},
		{"binary", map[string]string{"img.png": "x"}, "diff --git a/img.png b/img.png\nBinary files a/img.png and b/img.png differ\n", evo.ErrPatchUnsupported},
		{"git binary patch", map[string]string{"img.png": "x"}, "diff --git a/img.png b/img.png\nindex 1111111..2222222 100644\nGIT binary patch\nliteral 1\nIcmZpZ0000600IC2\n\nliteral 1\nIcmZpZ0000600IC2\n\n", evo.ErrPatchUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := workspace(t, tc.files)
			if err := apply(t, evo.Config{}, tc.diff, nil); !errors.Is(err, tc.want) {
				t.Fatalf("Patch = %v, want %v", err, tc.want)
			}
			for rel, body := range tc.files {
				if got := read(t, filepath.Join(dir, rel)); got != body {
					t.Fatalf("%s = %q after a rejected Patch, want untouched %q", rel, got, body)
				}
			}
			if entries, _ := os.ReadDir(dir); len(entries) != len(tc.files) {
				t.Fatalf("a rejected Patch left %d entries in the workspace, want %d: %v", len(entries), len(tc.files), entries)
			}
		})
	}
}

func TestPatchRejectsUnsafeOrOutOfScopePaths(t *testing.T) {
	cases := []struct {
		name string
		path func(t *testing.T) string // the diff path; outside the workspace
	}{
		{"parent traversal", func(*testing.T) string { return "../escape.txt" }},
		{"nested traversal", func(*testing.T) string { return "docs/../../escape.txt" }},
		{"absolute path", func(t *testing.T) string { return filepath.ToSlash(filepath.Join(t.TempDir(), "escape.txt")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path(t)
			dir := workspace(t, map[string]string{})
			diff := "diff --git a/" + path + " b/" + path + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + path + "\n@@ -0,0 +1 @@\n+x\n"
			if err := apply(t, evo.Config{}, diff, nil); !errors.Is(err, evo.ErrPatchUnsafePath) {
				t.Fatalf("Patch = %v, want ErrPatchUnsafePath", err)
			}
			if exists(filepath.Join(filepath.Dir(dir), "escape.txt")) || (filepath.IsAbs(path) && exists(path)) {
				t.Fatalf("an out-of-scope path was written")
			}
		})
	}
}

// Every affected path is identified before anything commits: an unsafe path
// in a later section stops an earlier, safe section too.
func TestPatchRejectsAnUnsafePathBeforeCommittingAnything(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
	diff := modifyDiff + "--- /dev/null\n+++ b/../escape.txt\n@@ -0,0 +1 @@\n+x\n"
	if err := apply(t, evo.Config{}, diff, nil); !errors.Is(err, evo.ErrPatchUnsafePath) {
		t.Fatalf("Patch = %v, want ErrPatchUnsafePath", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "draft\n" {
		t.Fatalf("notes.txt = %q; a safe section committed before an unsafe one was rejected", got)
	}
	if exists(filepath.Join(filepath.Dir(dir), "escape.txt")) {
		t.Fatalf("an out-of-scope path was written")
	}
}

func TestPatchRejectsPathsThroughASymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	plant(t, outside, map[string]string{"notes.txt": "draft\n"})
	dir := workspace(t, map[string]string{})
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	const diff = "--- a/link/notes.txt\n+++ b/link/notes.txt\n@@ -1 +1 @@\n-draft\n+final\n"
	if err := apply(t, evo.Config{}, diff, nil); !errors.Is(err, evo.ErrPatchUnsafePath) {
		t.Fatalf("Patch = %v, want ErrPatchUnsafePath", err)
	}
	if got := read(t, filepath.Join(outside, "notes.txt")); got != "draft\n" {
		t.Fatalf("Patch wrote through a symlinked directory: %q", got)
	}
}

// An edit that lands after the Task starts but before Patch reads is the
// simple case: the preimage no longer matches. The race between Patch's read
// and its commit is TestPatchNeverLosesAConcurrentFileWrite.
func TestPatchDetectsAConcurrentEdit(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
	err := apply(t, evo.Config{}, modifyDiff, func() {
		plant(t, dir, map[string]string{"notes.txt": "edited meanwhile\n"})
	})
	if !errors.Is(err, evo.ErrPatchDoesNotApply) && !errors.Is(err, evo.ErrPatchStale) {
		t.Fatalf("Patch = %v, want ErrPatchDoesNotApply or ErrPatchStale", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "edited meanwhile\n" {
		t.Fatalf("Patch overwrote a concurrent edit: %q", got)
	}
}

func TestPatchCommitsThroughFileMachinery(t *testing.T) {
	// A Patch-written file verifies as the File it establishes and is
	// atomically replaced (new inode), exactly as File.Write does.
	dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
	path := filepath.Join(dir, "notes.txt")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(t, evo.Config{}, modifyDiff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatalf("Patch edited notes.txt in place; want atomic replacement like File.Write")
	}
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	task := out.Task("verify").Define(func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("final\n")}.Verify(ctx)
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("Verify of the patched file: %v", err)
	}
	_ = out.Finish()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("Patch left temporaries in the workspace: %v", entries)
	}
}

func TestPatchRequiresATaskContext(t *testing.T) {
	workspace(t, map[string]string{"notes.txt": "draft\n"})
	if err := evo.Patch(context.Background(), []byte(modifyDiff)); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Patch outside a Task = %v, want ErrNoTaskContext", err)
	}
}

func TestPatchUnderDryRunMutatesNothing(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n", "docs/README.md": "docs\n"})
	if err := apply(t, evo.Config{DryRun: true}, multiFileDiff, nil); err != nil {
		t.Fatalf("dry-run Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "draft\n" {
		t.Fatalf("dry-run Patch changed notes.txt to %q", got)
	}
	if exists(filepath.Join(dir, "new.txt")) {
		t.Fatalf("dry-run Patch created new.txt")
	}
}

func TestPatchDryRunStillValidatesApplicability(t *testing.T) {
	workspace(t, map[string]string{"notes.txt": "other\n"})
	if err := apply(t, evo.Config{DryRun: true}, modifyDiff, nil); !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("dry-run Patch of a non-applicable diff = %v, want ErrPatchDoesNotApply", err)
	}
}

func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestPatchAppliesEveryHunkOfAFile(t *testing.T) {
	const diff = `--- a/list.txt
+++ b/list.txt
@@ -1,2 +1,2 @@
-one
+ONE
 two
@@ -5,2 +5,2 @@
 five
-six
+SIX
`
	dir := workspace(t, map[string]string{"list.txt": "one\ntwo\nthree\nfour\nfive\nsix\n"})
	if err := apply(t, evo.Config{}, diff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "list.txt")); got != "ONE\ntwo\nthree\nfour\nfive\nSIX\n" {
		t.Fatalf("list.txt = %q", got)
	}
}

// Standard unified-diff semantics: "\ No newline at end of file" is honoured
// in both directions.
func TestPatchHonoursNoNewlineAtEndOfFile(t *testing.T) {
	const addNewline = "--- a/tail.txt\n+++ b/tail.txt\n@@ -1 +1 @@\n-last\n\\ No newline at end of file\n+last\n"
	dir := workspace(t, map[string]string{"tail.txt": "last"})
	if err := apply(t, evo.Config{}, addNewline, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "tail.txt")); got != "last\n" {
		t.Fatalf("tail.txt = %q, want %q", got, "last\n")
	}

	const createWithout = "diff --git a/bare.txt b/bare.txt\nnew file mode 100644\n--- /dev/null\n+++ b/bare.txt\n@@ -0,0 +1 @@\n+bare\n\\ No newline at end of file\n"
	if err := apply(t, evo.Config{}, createWithout, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "bare.txt")); got != "bare" {
		t.Fatalf("bare.txt = %q, want %q", got, "bare")
	}
}

// A content-only hunk keeps the existing mode, as File does with Mode 0.
func TestPatchModifyPreservesMode(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
	path := filepath.Join(dir, "notes.txt")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := apply(t, evo.Config{}, modifyDiff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := perm(t, path); got != 0o755 {
		t.Fatalf("notes.txt mode = %v after a content-only Patch, want 0755", got)
	}
}

func TestPatchModeChangeClearsExecutable(t *testing.T) {
	const diff = "diff --git a/run.sh b/run.sh\nold mode 100755\nnew mode 100644\n"
	dir := workspace(t, map[string]string{"run.sh": "echo one\n"})
	path := filepath.Join(dir, "run.sh")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := apply(t, evo.Config{}, diff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := perm(t, path); got&0o111 != 0 {
		t.Fatalf("run.sh mode = %v, want not executable", got)
	}
	if got := read(t, path); got != "echo one\n" {
		t.Fatalf("a mode-only diff changed content: %q", got)
	}
}

func TestPatchCreateHonoursTheNewFileMode(t *testing.T) {
	const diff = "diff --git a/tool.sh b/tool.sh\nnew file mode 100755\n--- /dev/null\n+++ b/tool.sh\n@@ -0,0 +1 @@\n+echo hi\n"
	dir := workspace(t, map[string]string{})
	if err := apply(t, evo.Config{}, diff+createDiff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := perm(t, filepath.Join(dir, "tool.sh")); got&0o111 == 0 {
		t.Fatalf("tool.sh mode = %v, want executable (new file mode 100755)", got)
	}
	if got := perm(t, filepath.Join(dir, "new.txt")); got != 0o644 {
		t.Fatalf("new.txt mode = %v, want 0644 (new file mode 100644)", got)
	}
}

// Linearizability of Patch against a coordinated File.Write in a sibling Task.
// The writer always succeeds. If it lands first, Patch's preimage is gone and
// Patch must fail; if Patch lands first, the writer overwrites it. Either way
// the file ends holding the writer's bytes. "final\n" means Patch committed
// over an edit made between its read and its commit: a lost update.
func TestPatchNeverLosesAConcurrentFileWrite(t *testing.T) {
	const rounds = 40
	for round := range rounds {
		dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
		path := filepath.Join(dir, "notes.txt")
		out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
		group := out.Group("race")
		var patchErr, writeErr error
		group.Task("patch").Define(func(ctx context.Context) error {
			patchErr = evo.Patch(ctx, []byte(modifyDiff))
			return patchErr
		})
		group.Task("write").Define(func(ctx context.Context) error {
			writeErr = evo.File{Path: path, Content: evo.Bytes("edited\n")}.Write(ctx)
			return writeErr
		})
		_ = group.Wait()
		_ = out.Finish()
		if writeErr != nil {
			t.Fatalf("round %d: File.Write: %v", round, writeErr)
		}
		if patchErr != nil && !errors.Is(patchErr, evo.ErrPatchStale) && !errors.Is(patchErr, evo.ErrPatchDoesNotApply) {
			t.Fatalf("round %d: Patch = %v, want nil, ErrPatchStale, or ErrPatchDoesNotApply", round, patchErr)
		}
		if got := read(t, path); got != "edited\n" {
			t.Fatalf("round %d: notes.txt = %q (Patch err %v); Patch overwrote a concurrent File.Write", round, got, patchErr)
		}
	}
}

// Diff paths resolve against the working directory when the Run starts, not
// whatever it is when Patch is called.
func TestPatchResolvesPathsAgainstTheRunStartDirectory(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n"})
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	elsewhere := t.TempDir()
	plant(t, elsewhere, map[string]string{"notes.txt": "draft\n"})
	t.Chdir(elsewhere)
	err := out.Task("patch").Define(func(ctx context.Context) error {
		return evo.Patch(ctx, []byte(modifyDiff))
	}).Wait()
	_ = out.Finish()
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "notes.txt")); got != "final\n" {
		t.Fatalf("Run-start notes.txt = %q, want %q", got, "final\n")
	}
	if got := read(t, filepath.Join(elsewhere, "notes.txt")); got != "draft\n" {
		t.Fatalf("Patch followed a later chdir: %q", got)
	}
}

func TestPatchHonoursCancellation(t *testing.T) {
	dir := workspace(t, map[string]string{"notes.txt": "draft\n", "docs/README.md": "docs\n"})
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	var got error
	_ = out.Task("patch").Define(func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		got = evo.Patch(cctx, []byte(multiFileDiff))
		return got
	}).Wait()
	_ = out.Finish()
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("Patch with a cancelled ctx = %v, want context.Canceled", got)
	}
	if body := read(t, filepath.Join(dir, "notes.txt")); body != "draft\n" {
		t.Fatalf("cancelled Patch changed notes.txt to %q", body)
	}
	if exists(filepath.Join(dir, "new.txt")) {
		t.Fatalf("cancelled Patch created new.txt")
	}
}

func TestPatchUnderDryRunDeletesAndRenamesNothing(t *testing.T) {
	dir := workspace(t, map[string]string{"gone.txt": "bye\n", "old.txt": "same\n"})
	if err := apply(t, evo.Config{DryRun: true}, deleteDiff+renameDiff, nil); err != nil {
		t.Fatalf("dry-run Patch: %v", err)
	}
	if got := read(t, filepath.Join(dir, "gone.txt")); got != "bye\n" {
		t.Fatalf("dry-run Patch changed gone.txt: %q", got)
	}
	if got := read(t, filepath.Join(dir, "old.txt")); got != "same\n" {
		t.Fatalf("dry-run Patch changed old.txt: %q", got)
	}
	if exists(filepath.Join(dir, "renamed.txt")) {
		t.Fatalf("dry-run Patch created renamed.txt")
	}
}

// A created file and a deleted file commit through File too: the created
// file verifies as its File, and neither form leaves temporaries.
func TestPatchCreateAndDeleteCommitThroughFileMachinery(t *testing.T) {
	dir := workspace(t, map[string]string{"gone.txt": "bye\n"})
	if err := apply(t, evo.Config{}, createDiff+deleteDiff, nil); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	err := out.Task("verify").Define(func(ctx context.Context) error {
		return evo.File{Path: filepath.Join(dir, "new.txt"), Content: evo.Bytes("first\nsecond\n")}.Verify(ctx)
	}).Wait()
	_ = out.Finish()
	if err != nil {
		t.Fatalf("Verify of the created file: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "new.txt" {
		t.Fatalf("workspace = %v, want exactly new.txt", entries)
	}
}
