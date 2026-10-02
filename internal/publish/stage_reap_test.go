//go:build unix

package publish

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	reapChildEnv  = "EVO_REAP_CHILD_DEST"
	reapChildTest = "TestReapChildStagesThenDies"
)

// TestReapChildStagesThenDies is the child half of the crash test: run on
// its own, it stages a tree apart and dies without committing or discarding.
func TestReapChildStagesThenDies(t *testing.T) {
	dest := os.Getenv(reapChildEnv)
	if dest == "" {
		t.Skip("child half of the reaper crash test")
	}
	if _, err := StageTree(context.Background(), dest, 0, func(_ context.Context, root string) error {
		return os.WriteFile(filepath.Join(root, "f"), []byte("x"), 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func crashedStage(t *testing.T, cache string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+reapChildTest+"$")
	cmd.Env = append(os.Environ(), reapChildEnv+"="+filepath.Join(t.TempDir(), "dest"),
		"HOME="+cache, "XDG_CACHE_HOME="+cache)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	root := filepath.Join(userCache(t, cache), stagingRootName)
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("child left %v (err %v), want exactly one orphan", entries, err)
	}
	return filepath.Join(root, entries[0].Name())
}

func userCache(t *testing.T, home string) string {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", home)
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReaperRemovesACrashedStageOnlyAfterTheGraceWindow(t *testing.T) {
	orphan := crashedStage(t, t.TempDir())
	root := filepath.Dir(orphan)
	now := time.Now()

	if got := reapOrphanedStages(root, now, stageReapGrace); got != 0 {
		t.Fatalf("reaped %d inside the grace window, want 0", got)
	}
	if got := reapOrphanedStages(root, now.Add(stageReapGrace+time.Minute), stageReapGrace); got != 1 {
		t.Fatalf("reaped %d past the grace window, want 1", got)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan still present: %v", err)
	}
}

func TestReaperKeepsAStageWhoseLeaseIsHeld(t *testing.T) {
	orphan := crashedStage(t, t.TempDir())
	root := filepath.Dir(orphan)
	live := filepath.Join(root, filepath.Base(stagingName(filepath.Join(t.TempDir(), "live"))))
	if err := os.Mkdir(live, 0o700); err != nil {
		t.Fatal(err)
	}
	lease, ok, err := tryLease(live)
	if err != nil || !ok {
		t.Fatalf("lease live stage: ok=%v err=%v", ok, err)
	}
	defer func() { _ = lease.release() }()

	past := time.Now().Add(2 * stageReapGrace)
	if got := reapOrphanedStages(root, past, stageReapGrace); got != 1 {
		t.Fatalf("reaped %d, want only the orphan", got)
	}
	if _, err := os.Lstat(live); err != nil {
		t.Fatalf("live-lease stage was removed: %v", err)
	}
}

func TestReaperRunsOncePerIntervalPerProcess(t *testing.T) {
	root := filepath.Join(userCache(t, t.TempDir()), "none")
	clock := time.Now()
	defer func(f func() time.Time, at time.Time) { reapClock = f; lastReap.at = at }(reapClock, lastReap.at)
	reapClock = func() time.Time { return clock }
	lastReap.at = time.Time{}

	reapOrphanedStagesOnce(root)
	first := lastReap.at
	clock = clock.Add(stageReapInterval / 2)
	reapOrphanedStagesOnce(root)
	if !lastReap.at.Equal(first) {
		t.Fatal("reaped again inside the interval")
	}
	clock = clock.Add(stageReapInterval)
	reapOrphanedStagesOnce(root)
	if lastReap.at.Equal(first) {
		t.Fatal("did not reap after the interval")
	}
}
