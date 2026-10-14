package change

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
)

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

// patchRun is a Run over the real disk whose workspace is a temporary
// directory holding files, and the context of a Define callback under it.
func patchRun(t *testing.T, files map[string]string) (string, *Run, context.Context, *fakeHost) {
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
	host := newFakeHost(t.TempDir())
	run, ctx := scopedRun(t, host)
	run.PinWorkspace(dir)
	return dir, run, ctx, host
}

// assertDesired fails unless got is the desired state want describes.
func assertDesired(t *testing.T, got, want desiredFile) {
	t.Helper()
	if got.target != want.target || string(got.contents) != string(want.contents) || got.mode != want.mode {
		t.Fatalf("desired file = {%+v %q %v}, want {%+v %q %v}",
			got.target, got.contents, got.mode, want.target, want.contents, want.mode)
	}
}

// assertBasisMatchesFSPath proves the recorded Basis is the source's
// current FSPath identity, so a later revalidation compares like with like.
func assertBasisMatchesFSPath(t *testing.T, f desiredFile) {
	t.Helper()
	want, err := freshness.FSPath(f.target.path()).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.basis != want {
		t.Fatalf("Basis for %s = %+v, want FSPath identity %+v", f.target.path(), f.basis, want)
	}
}

func TestPatchOneFileDerivesDesiredStateWithSourceBasis(t *testing.T) {
	dir, _, ctx, _ := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n"})

	set, err := Patch(ctx, []byte(modifyGreetingDiff))

	if err != nil || len(set.files) != 1 {
		t.Fatalf("Patch = (%d files, %v), want one desired file", len(set.files), err)
	}
	assertDesired(t, set.files[0], desiredFile{
		target: workspaceFile{root: dir, rel: "greeting.txt"}, contents: []byte("hello\nthere\n"),
	})
	assertBasisMatchesFSPath(t, set.files[0])
}

func TestPatchMultiFileDerivesEachDesiredStateWithSourceBasis(t *testing.T) {
	dir, _, ctx, _ := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n", "docs/README.md": "docs\n"})

	set, err := Patch(ctx, []byte(modifyGreetingDiff+createDocDiff))

	if err != nil || len(set.files) != 2 {
		t.Fatalf("Patch = (%d files, %v), want two desired files", len(set.files), err)
	}
	assertDesired(t, set.files[0], desiredFile{
		target: workspaceFile{root: dir, rel: "greeting.txt"}, contents: []byte("hello\nthere\n"),
	})
	assertDesired(t, set.files[1], desiredFile{
		target: workspaceFile{root: dir, rel: filepath.Join("docs", "new.md")}, contents: []byte("# New\n"), mode: 0o644,
	})
	for _, f := range set.files {
		assertBasisMatchesFSPath(t, f)
	}
	created := filepath.Join(dir, "docs", "new.md")
	if set.files[1].basis != freshness.ObservedMissing(created) {
		t.Fatalf("a created file's Basis must be its observed absence, got %+v", set.files[1].basis)
	}
}

func TestPatchDerivationMutatesNothing(t *testing.T) {
	dir, _, ctx, host := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n"})

	if _, err := Patch(ctx, []byte(modifyGreetingDiff)); err != nil {
		t.Fatalf("Patch = %v", err)
	}

	if got := readFile(t, filepath.Join(dir, "greeting.txt")); got != "hello\nworld\n" {
		t.Errorf("greeting.txt = %q, want it untouched", got)
	}
	if len(host.recorded()) != 0 {
		t.Errorf("Patch recorded %v, want nothing: deriving is not a change", host.recorded())
	}
}

func TestPatchRefusesASourceTheDiffDoesNotMatch(t *testing.T) {
	cases := map[string]string{
		"context differs":      modifyGreetingDiff,
		"create over existing": "--- /dev/null\n+++ b/greeting.txt\n@@ -0,0 +1 @@\n+x\n",
		"modify missing":       "--- a/absent.txt\n+++ b/absent.txt\n@@ -1 +1 @@\n-a\n+b\n",
	}
	for name, diff := range cases {
		_, _, ctx, _ := patchRun(t, map[string]string{"greeting.txt": "hello\nplanet\n"})
		if _, err := Patch(ctx, []byte(diff)); !errors.Is(err, ErrPatchDoesNotApply) {
			t.Errorf("%s: Patch = %v, want ErrPatchDoesNotApply", name, err)
		}
	}
}

func TestFilesCommitsADerivedSetAndRefusesAStaleOne(t *testing.T) {
	dir, _, ctx, host := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	set, err := Patch(ctx, []byte(modifyGreetingDiff))
	if err != nil {
		t.Fatalf("Patch = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte("changed meanwhile\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = Files(ctx, set)

	if !errors.Is(err, ErrStaleBasis) {
		t.Fatalf("Files over a changed source = %v, want ErrStaleBasis", err)
	}
	if got := readFile(t, filepath.Join(dir, "greeting.txt")); got != "changed meanwhile\n" {
		t.Errorf("greeting.txt = %q, the concurrent edit was overwritten", got)
	}
	if len(host.recorded()) != 0 {
		t.Errorf("a refused Files recorded %v", host.recorded())
	}
}

func TestFilesWritesTheDerivedState(t *testing.T) {
	dir, _, ctx, host := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	set, err := Patch(ctx, []byte(modifyGreetingDiff))
	if err != nil {
		t.Fatalf("Patch = %v", err)
	}

	if err := Files(ctx, set); err != nil {
		t.Fatalf("Files = %v", err)
	}

	if got := readFile(t, filepath.Join(dir, "greeting.txt")); got != "hello\nthere\n" {
		t.Errorf("greeting.txt = %q, want the patched contents", got)
	}
	if got := host.recorded(); len(got) != 1 || got[0].Verb() != "write" {
		t.Errorf("recorded %v, want one write", got)
	}
}

func TestApplyPatchCreatesModifiesAndRemoves(t *testing.T) {
	dir, _, ctx, host := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n", "old.txt": "hello\nworld\n"})
	diff := modifyGreetingDiff + createDocDiff + `diff --git a/old.txt b/old.txt
deleted file mode 100644
--- a/old.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-hello
-world
`

	if err := ApplyPatch(ctx, []byte(diff)); err != nil {
		t.Fatalf("ApplyPatch = %v", err)
	}

	if got := readFile(t, filepath.Join(dir, "greeting.txt")); got != "hello\nthere\n" {
		t.Errorf("greeting.txt = %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "docs", "new.md")); got != "# New\n" {
		t.Errorf("docs/new.md = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old.txt still exists: %v", err)
	}
	if got := host.recorded(); len(got) != 3 {
		t.Errorf("recorded %d entries, want 3 (two writes, one remove)", len(got))
	}
}

func TestApplyPatchDryRunValidatesAndChangesNothing(t *testing.T) {
	dir, _, ctx, host := patchRun(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	host.applies = false

	if err := ApplyPatch(ctx, []byte(modifyGreetingDiff+createDocDiff)); err != nil {
		t.Fatalf("ApplyPatch = %v", err)
	}

	if got := readFile(t, filepath.Join(dir, "greeting.txt")); got != "hello\nworld\n" {
		t.Errorf("greeting.txt = %q, want it untouched", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a planned Patch created docs/: %v", err)
	}
	if got := host.recorded(); len(got) != 2 {
		t.Errorf("recorded %d planned entries, want 2", len(got))
	}
}

func TestApplyPatchRefusesAPathBeyondASymlink(t *testing.T) {
	dir, _, ctx, _ := patchRun(t, nil)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}

	err := ApplyPatch(ctx, []byte("--- /dev/null\n+++ b/escape/pwned.txt\n@@ -0,0 +1 @@\n+x\n"))

	if err == nil {
		t.Fatal("ApplyPatch through a symlinked directory succeeded")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "pwned.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("ApplyPatch wrote outside the workspace: %v", statErr)
	}
}

func TestApplyPatchRefusesStaleSourceAtCommit(t *testing.T) {
	_, _, ctx, _ := patchRun(t, map[string]string{"greeting.txt": "hello\nplanet\n"})

	err := ApplyPatch(ctx, []byte(modifyGreetingDiff))

	if !errors.Is(err, ErrPatchDoesNotApply) {
		t.Fatalf("ApplyPatch over different context = %v, want ErrPatchDoesNotApply", err)
	}
}
