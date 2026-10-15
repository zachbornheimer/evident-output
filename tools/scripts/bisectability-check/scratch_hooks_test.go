package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The tool proves commits in isolation, so no hook the developer's git
// configuration installs may run in its scratch checkouts: a hook that fails
// there fails the proof for a reason that is not the commit's.
func TestScratchCheckout_RunsNoInstalledHooks(t *testing.T) {
	repo := newRepoWithTwoCommits(t)
	hooks := t.TempDir()
	marker := filepath.Join(t.TempDir(), "hook-ran")
	script := "#!/bin/sh\ntouch '" + marker + "'\n"
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo.dir, "config", "core.hooksPath", hooks)

	worktree, cleanup, err := newScratchWorktree(repo.dir)
	if err != nil {
		t.Fatalf("newScratchWorktree: %v", err)
	}
	t.Cleanup(cleanup)
	if err := checkoutDetached(worktree, repo.first); err != nil {
		t.Fatalf("checkoutDetached: %v", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Error("an installed post-checkout hook ran in the scratch checkout")
	}
}
