package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A parent commit swapped the directory the stage lived in, so the stage
// is gone from its path. The commit says so, publishes nothing, and leaves
// the new parent tree exactly as it found it, including a directory at a
// path staging had created. Only a stage prepared beside its destination
// (the staging root on another volume) can be carried away.
func TestCommitOfAStageCarriedAwayIsStagedGone(t *testing.T) {
	defer InjectFaults(Faults{StageBeside: true})()
	parent := filepath.Join(t.TempDir(), "parent")
	dest := filepath.Join(parent, "made", "pkg")
	plantTree(t, filepath.Join(parent, "keep"), originalFiles)
	s := stageTree(t, dest, replacementFiles)
	if err := os.Rename(parent, parent+".swapped-out"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(parent, "made"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(context.Background(), Guard{}); !errors.Is(err, ErrStagedGone) {
		t.Fatalf("Commit of a carried-away stage = %v, want ErrStagedGone", err)
	}
	requireOnly(t, parent, "made")
	requireOnly(t, filepath.Join(parent, "made"))
}
