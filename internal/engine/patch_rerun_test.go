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
