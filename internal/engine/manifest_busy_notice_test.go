package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestManifestBusyNoticeIsPrintedOnceForBasisOnlyRun proves the busy notice
// is not specific to File/Exec: a Task that only declares Basis opens the
// manifest too, and degrading past a held lock must still say so, once.
func TestManifestBusyNoticeIsPrintedOnceForBasisOnlyRun(t *testing.T) {
	state := t.TempDir()
	holder := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = holder.Close() })
	if err := runFileTask(t, holder, "hold", FileSpec{Path: filepath.Join(t.TempDir(), "held.txt"), Contents: []byte("x")}); err != nil {
		t.Fatalf("holder run: %v", err)
	}

	input := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(input, []byte("in"), 0o600); err != nil {
		t.Fatalf("seed basis input: %v", err)
	}
	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })
	for _, name := range []string{"first", "second"} {
		task := out.Task(name).Basis(FileBasis(input))
		task.Define(func(ctx context.Context) error { return nil })
		_ = task.Wait()
	}
	notices := 0
	for _, f := range out.Snapshot().Facts {
		if f.Name == "manifest" {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("manifest busy notices = %d, want exactly 1", notices)
	}
}
