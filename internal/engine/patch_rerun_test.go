package engine

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestPatchRerunOfAppliedDiffIsSatisfied proves applying the same diff
// on a second Run is already satisfied, not a failure, for a
// modification and a creation alike.
func TestPatchRerunOfAppliedDiffIsSatisfied(t *testing.T) {
	dir, first := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	if err := runFilesTask(t, first, twoFileEditDiff, nil); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	second := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = second.Close() })
	second.mu.Lock()
	second.workspaceDir = dir
	second.mu.Unlock()
	if err := runFilesTask(t, second, twoFileEditDiff, nil); err != nil {
		t.Fatalf("run 2 = %v, want already satisfied", err)
	}
	if _, changed := effectObjects(second); len(changed) != 0 {
		t.Fatalf("run 2 changes = %v, want none", changed)
	}
	if got := readOrFatal(t, filepath.Join(dir, "greeting.txt")); got != "hello\nthere\n" {
		t.Fatalf("greeting.txt = %q", got)
	}
}

// TestPatchRerunRejectsASourceThatIsNotTheResult proves the rerun rule
// only accepts a source that already is the diff's result: greeting.txt
// is, but notes.txt holds other bytes, so creating it does not apply.
func TestPatchRerunRejectsASourceThatIsNotTheResult(t *testing.T) {
	_, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nthere\n", "notes.txt": "other\n"})
	set, err := runPatchTask(t, out, twoFileEditDiff)
	if !errors.Is(err, ErrPatchDoesNotApply) {
		t.Fatalf("err = %v (set %d files), want ErrPatchDoesNotApply", err, len(set.files))
	}
}

// duplicateInsertionDiff inserts a line equal to the line that already
// follows its context, so "head\na\na\n" matches both the diff's old side
// and its new side.
const duplicateInsertionDiff = "--- a/list.txt\n+++ b/list.txt\n@@ -2,1 +2,2 @@\n a\n+a\n"

// TestPatchAmbiguousFirstRunAppliesForward proves a source that matches
// both sides of a diff is patched forward on its first Run, as patch(1)
// and git apply do, and that the same Task's next Run (which reads its
// own recorded result from the manifest) is already satisfied instead of
// inserting the line again.
func TestPatchAmbiguousFirstRunAppliesForward(t *testing.T) {
	const want = "head\na\na\na\n"
	dir, first := patchWorkspace(t, map[string]string{"list.txt": "head\na\na\n"})
	state := first.cfg.stateDir
	if err := runFilesTask(t, first, duplicateInsertionDiff, nil); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if got := readOrFatal(t, filepath.Join(dir, "list.txt")); got != want {
		t.Fatalf("list.txt after run 1 = %q, want %q", got, want)
	}
	if _, changed := effectObjects(first); len(changed) != 1 {
		t.Fatalf("run 1 changes = %v, want one", changed)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close run 1: %v", err)
	}
	second := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = second.Close() })
	second.mu.Lock()
	second.workspaceDir = dir
	second.mu.Unlock()
	if err := runFilesTask(t, second, duplicateInsertionDiff, nil); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if got := readOrFatal(t, filepath.Join(dir, "list.txt")); got != want {
		t.Fatalf("list.txt after run 2 = %q, want %q (already satisfied)", got, want)
	}
	if _, changed := effectObjects(second); len(changed) != 0 {
		t.Fatalf("run 2 changes = %v, want none", changed)
	}
}

// swapToADiff turns "head\nb\na\n" into "head\na\na\n": a different diff
// whose result is exactly the source duplicateInsertionDiff then matches
// on both sides.
const swapToADiff = "--- a/list.txt\n+++ b/list.txt\n@@ -2,1 +2,1 @@\n-b\n+a\n"

// TestPatchAmbiguousSourceTrustsOnlyTheSameDiffsResult proves the
// ambiguous rerun rule asks whether this Task's previous Run applied this
// same diff, not merely whether it left these bytes: run 1 applies one
// diff, run 2 a different one whose old and new sides both match, and run
// 2 must still apply forward (git apply gives head/a/a/a).
func TestPatchAmbiguousSourceTrustsOnlyTheSameDiffsResult(t *testing.T) {
	dir, first := patchWorkspace(t, map[string]string{"list.txt": "head\nb\na\n"})
	state := first.cfg.stateDir
	if err := runFilesTask(t, first, swapToADiff, nil); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close run 1: %v", err)
	}
	second := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = second.Close() })
	second.mu.Lock()
	second.workspaceDir = dir
	second.mu.Unlock()
	if err := runFilesTask(t, second, duplicateInsertionDiff, nil); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if got := readOrFatal(t, filepath.Join(dir, "list.txt")); got != "head\na\na\na\n" {
		t.Fatalf("list.txt after run 2 = %q, want %q", got, "head\na\na\na\n")
	}
	if _, changed := effectObjects(second); len(changed) != 1 {
		t.Fatalf("run 2 changes = %v, want one", changed)
	}
}
