package tree_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish"
)

// pruneCrashChild swaps a staged tree over dest and dies inside the
// post-swap verification: after the atomic exchange, before the replaced
// original is cleaned up.
func pruneCrashChild(t *testing.T) {
	dest := os.Getenv(pruneDestEnv)
	staged, err := publish.StageTree(context.Background(), dest, 0, func(_ context.Context, root string) error {
		return os.WriteFile(filepath.Join(root, "index.js"), []byte("new"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = staged.Commit(context.Background(), publish.Guard{Verify: func(context.Context, string) error {
		os.Exit(0)
		return nil
	}})
	t.Fatal("commit returned; the crash hook never ran")
}

func TestPrune_CrashBetweenSwapAndCleanupLeavesARecoverableState(t *testing.T) {
	f := newPruneReplaceFixture(t)
	if err := pruneChild(t, pruneRoleEnv+"="+pruneRoleCrash, pruneDestEnv+"="+f.dest).Run(); err != nil {
		t.Fatalf("crashing child: %v", err)
	}
	// The destination is always one whole tree: the exchange is atomic.
	if got := onDisk(t, f.dest); !equalFiles(got, map[string]string{"index.js": "new"}) {
		t.Fatalf("destination after crash = %v, want the whole new tree", got)
	}
	// The original survives beside it, bound to this destination, hidden
	// from readers, and identifiable by the digest the caller planned.
	leftovers, err := publish.Leftovers(f.dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 1 {
		t.Fatalf("Leftovers = %v, want exactly the retained original", leftovers)
	}
	if got := pruneChecksum(t, leftovers[0]); got != f.expected {
		t.Fatalf("retained original digests to %s, want the planned %s", got, f.expected)
	}
	if !publish.IsStaging(filepath.Base(leftovers[0])) {
		t.Fatalf("retained original %s is visible to readers", leftovers[0])
	}
	// The kernel dropped the dead process's coordination.
	_ = pruneMustHold(t, f.dest).Release()
	// Restore: put the original back atomically.
	if err := os.RemoveAll(f.dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(leftovers[0], f.dest); err != nil {
		t.Fatal(err)
	}
	if got := pruneChecksum(t, f.dest); got != f.expected {
		t.Fatalf("restored destination digests to %s, want %s", got, f.expected)
	}
}
