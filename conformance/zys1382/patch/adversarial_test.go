package patch_test

// Adversarial hardening tests for evo.Patch. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// advWorkspace plants files in a temp dir and makes it the working
// directory diff paths resolve against.
func advWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

func advPatch(tb testing.TB, diff string) error {
	tb.Helper()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: tb.TempDir()})
	defer func() { _ = out.Close() }()
	err := out.Task("patch").Define(func(ctx context.Context) error {
		return evo.Patch(ctx, []byte(diff))
	}).Wait()
	_ = out.Finish()
	return err
}

func advContent(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func advAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s was written (stat err %v)", path, err)
	}
}

// advCreateDiff creates path with one line.
func advCreateDiff(path, line string) string {
	return fmt.Sprintf("--- /dev/null\n+++ b/%s\n@@ -0,0 +1 @@\n+%s\n", path, line)
}

func TestAdversarial_ParentTraversalRejected(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "a\n"})
	for _, path := range []string{"../escape.txt", "sub/../../escape2.txt"} {
		if err := advPatch(t, advCreateDiff(path, "x")); !errors.Is(err, evo.ErrPatchUnsafePath) {
			t.Fatalf("diff creating %q = %v, want ErrPatchUnsafePath", path, err)
		}
	}
	advAbsent(t, filepath.Join(filepath.Dir(ws), "escape.txt"))
	advAbsent(t, filepath.Join(filepath.Dir(ws), "escape2.txt"))
}

func TestAdversarial_AbsolutePathRejected(t *testing.T) {
	advWorkspace(t, nil)
	target := filepath.Join(t.TempDir(), "abs.txt")
	diff := fmt.Sprintf("--- /dev/null\n+++ %s\n@@ -0,0 +1 @@\n+x\n", target)
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchUnsafePath) {
		t.Fatalf("absolute path = %v, want ErrPatchUnsafePath", err)
	}
	advAbsent(t, target)
}

// Git CVE-2023-23946 / GNU patch CVE-2019-13636: a path beyond a symlinked
// directory writes outside the workspace.
func TestAdversarial_PathBeyondSymlinkRejected(t *testing.T) {
	ws := advWorkspace(t, nil)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(ws, "link")); err != nil {
		t.Fatal(err)
	}
	if err := advPatch(t, advCreateDiff("link/pwn.txt", "x")); !errors.Is(err, evo.ErrPatchUnsafePath) {
		t.Fatalf("path through a symlink = %v, want ErrPatchUnsafePath", err)
	}
	advAbsent(t, filepath.Join(outside, "pwn.txt"))
}

// Git CVE-2023-23946 exploit shape: one diff creates a symlink, a later
// section writes through it.
func TestAdversarial_SymlinkCreatedThenWrittenThroughRejected(t *testing.T) {
	advWorkspace(t, nil)
	outside := t.TempDir()
	diff := fmt.Sprintf("diff --git a/link b/link\nnew file mode 120000\n--- /dev/null\n+++ b/link\n@@ -0,0 +1 @@\n+%s\n\\ No newline at end of file\n", outside) +
		advCreateDiff("link/pwn.txt", "x")
	if err := advPatch(t, diff); err == nil {
		t.Fatal("diff creating a symlink and writing through it applied")
	}
	advAbsent(t, filepath.Join(outside, "pwn.txt"))
}

// Git CVE-2014-9390: .git paths (any case) plant hooks.
func TestAdversarial_GitDirectoryRejected(t *testing.T) {
	ws := advWorkspace(t, nil)
	for _, path := range []string{".git/hooks/post-checkout", ".GIT/hooks/post-checkout", "sub/.Git/config"} {
		if err := advPatch(t, advCreateDiff(path, "#!/bin/sh")); !errors.Is(err, evo.ErrPatchUnsafePath) {
			t.Fatalf("diff writing %q = %v, want ErrPatchUnsafePath", path, err)
		}
		advAbsent(t, filepath.Join(ws, filepath.FromSlash(path)))
	}
}

func TestAdversarial_AllOrNothingAcrossFiles(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "one\ntwo\n", "b.txt": "three\nfour\n"})
	diff := "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n" +
		"--- a/b.txt\n+++ b/b.txt\n@@ -1,2 +1,2 @@\n MISMATCH\n-four\n+FOUR\n"
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("partially applicable diff = %v, want ErrPatchDoesNotApply", err)
	}
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "one\ntwo\n" {
		t.Fatalf("first file committed although the second did not apply: %q", got)
	}
}

// GNU patch's default fuzz drops mismatching context lines; Evo must not.
func TestAdversarial_NoFuzzyApplication(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"c.txt": "alpha\nbeta\ngamma\ndelta\nepsilon\n"})
	diff := "--- a/c.txt\n+++ b/c.txt\n@@ -1,5 +1,5 @@\n alpha\n BETA-CHANGED\n-gamma\n+GAMMA\n delta\n EPSILON-CHANGED\n"
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("fuzzy hunk = %v, want ErrPatchDoesNotApply", err)
	}
	if got := advContent(t, filepath.Join(ws, "c.txt")); strings.Contains(got, "GAMMA") {
		t.Fatal("hunk applied with mismatching context")
	}
}

// GNU patch CVE-2018-1000156: ed-style scripts ran shell commands.
func TestAdversarial_EdScriptRejectedNotExecuted(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "a\n"})
	marker := filepath.Join(ws, "pwned")
	diff := "1a\nx\n.\nw a.txt\n!touch " + marker + "\nq\n"
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchMalformed) {
		t.Fatalf("ed script = %v, want ErrPatchMalformed", err)
	}
	advAbsent(t, marker)
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "a\n" {
		t.Fatalf("ed script modified a.txt: %q", got)
	}
}

func TestAdversarial_HunkCountMismatchRejected(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "one\ntwo\nthree\n"})
	diff := "--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n" // header promises 3 lines, body has 2
	if err := advPatch(t, diff); err == nil {
		t.Fatal("hunk whose header disagrees with its body applied")
	}
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "one\ntwo\nthree\n" {
		t.Fatalf("corrupt hunk changed the file: %q", got)
	}
}

func TestAdversarial_CreateOverExistingRejected(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "precious\n"})
	if err := advPatch(t, advCreateDiff("a.txt", "replacement")); !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("create over an existing file = %v, want ErrPatchDoesNotApply", err)
	}
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "precious\n" {
		t.Fatalf("existing file overwritten: %q", got)
	}
}

func TestAdversarial_DeleteRequiresExactPreimage(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "one\ntwo\nlocal edit\n"})
	diff := "--- a/a.txt\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-one\n-two\n"
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("delete with a stale preimage = %v, want ErrPatchDoesNotApply", err)
	}
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "one\ntwo\nlocal edit\n" {
		t.Fatalf("file changed: %q", got)
	}
}

func TestAdversarial_RenameOutOfScopeRejected(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "a\n"})
	diff := "diff --git a/a.txt b/../a.txt\nsimilarity index 100%\nrename from a.txt\nrename to ../a.txt\n"
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchUnsafePath) {
		t.Fatalf("rename out of the workspace = %v, want ErrPatchUnsafePath", err)
	}
	advAbsent(t, filepath.Join(filepath.Dir(ws), "a.txt"))
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "a\n" {
		t.Fatalf("source changed: %q", got)
	}
}

// Git only honors 100644/100755; a diff never grants setuid.
func TestAdversarial_SetuidModeNeverApplied(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"tool": "#!/bin/sh\n"})
	diff := "diff --git a/tool b/tool\nold mode 100644\nnew mode 104755\n"
	_ = advPatch(t, diff)
	info, err := os.Stat(filepath.Join(ws, "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
		t.Fatalf("diff applied mode %v with setuid/setgid", info.Mode())
	}
}

// Structural: re-applying an applied diff is a no-op with no rewrite.
func TestAdversarial_AlreadyAppliedIsNoOp(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "one\ntwo\n"})
	diff := "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	if err := advPatch(t, diff); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ws, "a.txt")
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := advPatch(t, diff); err != nil {
		t.Fatalf("re-applying an applied diff: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("re-applying an applied diff rewrote the file")
	}
}

func BenchmarkPatch_100Files(b *testing.B) {
	dir := b.TempDir()
	var forward, backward strings.Builder
	for i := range 100 {
		name := fmt.Sprintf("f%03d.txt", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep\nold\nkeep\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		fmt.Fprintf(&forward, "--- a/%s\n+++ b/%s\n@@ -1,3 +1,3 @@\n keep\n-old\n+new\n keep\n", name, name)
		fmt.Fprintf(&backward, "--- a/%s\n+++ b/%s\n@@ -1,3 +1,3 @@\n keep\n-new\n+old\n keep\n", name, name)
	}
	b.Chdir(dir)
	diffs := [2]string{forward.String(), backward.String()}
	for i := 0; b.Loop(); i++ {
		if err := advPatch(b, diffs[i%2]); err != nil {
			b.Fatal(err)
		}
	}
}

// CWE-59: a target that is itself a symlink must never redirect the write.
// Atomic replacement may replace the link or refuse; the link's target
// outside the workspace stays untouched either way.
func TestAdversarial_FinalSymlinkNeverWrittenThrough(t *testing.T) {
	ws := advWorkspace(t, nil)
	outside := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(outside, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "a.txt")); err != nil {
		t.Fatal(err)
	}
	_ = advPatch(t, "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+pwned\n")
	if got := advContent(t, outside); got != "one\n" {
		t.Fatalf("Patch wrote through a symlinked target: %q", got)
	}
}

// Byte-exact: CRLF lines match CRLF hunks and stay CRLF.
func TestAdversarial_CRLFLinesPreserved(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"w.txt": "a\r\nb\r\n"})
	if err := advPatch(t, "--- a/w.txt\n+++ b/w.txt\n@@ -1,2 +1,2 @@\n a\r\n-b\r\n+c\r\n"); err != nil {
		t.Fatalf("CRLF diff: %v", err)
	}
	if got := advContent(t, filepath.Join(ws, "w.txt")); got != "a\r\nc\r\n" {
		t.Fatalf("w.txt = %q, want %q", got, "a\r\nc\r\n")
	}
}

// A hunk header with out-of-range numbers is malformed, never a panic or a
// huge allocation.
func TestAdversarial_OverflowingHunkHeaderRejected(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"a.txt": "a\n"})
	diff := "--- a/a.txt\n+++ b/a.txt\n@@ -99999999999999999999,1 +1,99999999999999999999 @@\n-a\n+b\n"
	if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchMalformed) {
		t.Fatalf("overflowing hunk header = %v, want ErrPatchMalformed", err)
	}
	if got := advContent(t, filepath.Join(ws, "a.txt")); got != "a\n" {
		t.Fatalf("a.txt changed: %q", got)
	}
}

// NUL in a path truncates at the syscall boundary in C tools.
func TestAdversarial_NULInPathRejected(t *testing.T) {
	ws := advWorkspace(t, nil)
	err := advPatch(t, advCreateDiff("ok.txt\x00../../escape.txt", "x"))
	if !errors.Is(err, evo.ErrPatchUnsafePath) && !errors.Is(err, evo.ErrPatchMalformed) {
		t.Fatalf("NUL in path = %v, want ErrPatchUnsafePath or ErrPatchMalformed", err)
	}
	advAbsent(t, filepath.Join(ws, "ok.txt"))
	advAbsent(t, filepath.Join(filepath.Dir(ws), "escape.txt"))
}

// A diff naming a directory as a file never replaces or deletes the directory.
func TestAdversarial_DirectoryTargetRejected(t *testing.T) {
	ws := advWorkspace(t, map[string]string{"dir/keep.txt": "keep\n"})
	for name, diff := range map[string]string{
		"create": advCreateDiff("dir", "x"),
		"delete": "--- a/dir\n+++ /dev/null\n@@ -1 +0,0 @@\n-keep\n",
	} {
		if err := advPatch(t, diff); !errors.Is(err, evo.ErrPatchDoesNotApply) {
			t.Fatalf("%s over a directory = %v, want ErrPatchDoesNotApply", name, err)
		}
		if got := advContent(t, filepath.Join(ws, "dir", "keep.txt")); got != "keep\n" {
			t.Fatalf("%s over a directory changed its contents: %q", name, got)
		}
	}
}

// One hunk deep in a large file: application cost must stay linear.
func BenchmarkPatch_OneHunkIn100kLineFile(b *testing.B) {
	dir := b.TempDir()
	var body strings.Builder
	for i := range 100_000 {
		fmt.Fprintf(&body, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(body.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	const at = 90_000
	hunk := func(from, to string) string {
		return fmt.Sprintf("--- a/big.txt\n+++ b/big.txt\n@@ -%d,3 +%d,3 @@\n line %d\n-%s\n+%s\n line %d\n", at, at, at-1, from, to, at+1)
	}
	b.Chdir(dir)
	old := fmt.Sprintf("line %d", at)
	diffs := [2]string{hunk(old, "changed"), hunk("changed", old)}
	for i := 0; b.Loop(); i++ {
		if err := advPatch(b, diffs[i%2]); err != nil {
			b.Fatal(err)
		}
	}
}
