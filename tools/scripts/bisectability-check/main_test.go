package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFirstNonEmpty proves the flag/env/default precedence order.
func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "default"); got != "default" {
		t.Fatalf("firstNonEmpty fell through to %q, want default", got)
	}
	if got := firstNonEmpty("arg", "env", "default"); got != "arg" {
		t.Fatalf("firstNonEmpty = %q, want the first non-empty value", got)
	}
	if got := firstNonEmpty("", "env", "default"); got != "env" {
		t.Fatalf("firstNonEmpty = %q, want the second value once the first is empty", got)
	}
}

// TestArgAt proves an out-of-range index reports empty rather than panicking
// — run(args) relies on this to make BASE and HEAD both optional.
func TestArgAt(t *testing.T) {
	args := []string{"only-base"}
	if got := argAt(args, 0); got != "only-base" {
		t.Fatalf("argAt(0) = %q, want %q", got, "only-base")
	}
	if got := argAt(args, 1); got != "" {
		t.Fatalf("argAt(1) = %q, want empty for a missing HEAD argument", got)
	}
}

// TestFirstParentCommits_KnownBrokenCommit is this tool's own red-first
// proof: 48baff0 is a real, already-merged commit in this repo's history
// that does not compile on its own (internal/engine/file.go references
// settleOutputBarrier/openOutputBarrierLocked before a later commit in the
// same range adds them) — bisectability-check must name exactly that
// commit as the one that fails to build.
func TestFirstParentCommits_KnownBrokenCommit(t *testing.T) {
	if !hasCommit(t, "48baff0") {
		t.Skip("48baff0 not present in this checkout's history")
	}
	commits, err := firstParentCommits(".", "733d833", "48baff0")
	if err != nil {
		t.Fatalf("firstParentCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("firstParentCommits(733d833, 48baff0) = %v, want exactly the one known-broken commit", commits)
	}

	worktree, cleanup, err := newScratchWorktree(".")
	if err != nil {
		t.Fatalf("newScratchWorktree: %v", err)
	}
	t.Cleanup(cleanup)

	if err := checkoutDetached(worktree, commits[0]); err != nil {
		t.Fatalf("checkoutDetached: %v", err)
	}
	if _, err := buildAll(worktree); err == nil {
		t.Fatal("buildAll succeeded on the known-broken commit 48baff0; expected a build failure")
	}
}

func hasCommit(t *testing.T, ref string) bool {
	t.Helper()
	_, err := runGit("", "cat-file", "-e", ref)
	return err == nil
}

// TestScratchCheckout_IgnoresInheritedRepositoryLocation is the regression
// proof for the pre-push hook defect: git exports GIT_DIR, GIT_WORK_TREE and
// GIT_INDEX_FILE to hooks, and a checkout that inherited them moved the
// pushing repository's HEAD and index while its files landed in the scratch
// directory. The variables point at a throwaway repo here, never this one.
func TestScratchCheckout_IgnoresInheritedRepositoryLocation(t *testing.T) {
	hostile := newRepoWithTwoCommits(t)
	caller := filepath.Join(t.TempDir(), "caller")
	mustGit(t, "", "clone", "--quiet", hostile.dir, caller)

	t.Setenv("GIT_DIR", filepath.Join(hostile.dir, ".git"))
	t.Setenv("GIT_WORK_TREE", hostile.dir)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(hostile.dir, ".git", "index"))

	worktree, cleanup, err := newScratchWorktree(caller)
	if err != nil {
		t.Fatalf("newScratchWorktree: %v", err)
	}
	t.Cleanup(cleanup)
	if err := checkoutDetached(worktree, hostile.first); err != nil {
		t.Fatalf("checkoutDetached: %v", err)
	}

	if got := mustGit(t, hostile.dir, "rev-parse", "HEAD"); got != hostile.second {
		t.Errorf("hostile repo HEAD = %s, want unmoved %s", got, hostile.second)
	}
	if got := mustGit(t, hostile.dir, "ls-files", "--stage"); got != hostile.index {
		t.Errorf("hostile repo index changed:\n%s\nwant\n%s", got, hostile.index)
	}
	if got := mustGit(t, worktree, "rev-parse", "HEAD"); got != hostile.first {
		t.Errorf("scratch worktree HEAD = %s, want %s", got, hostile.first)
	}
}

type throwawayRepo struct{ dir, first, second, index string }

func newRepoWithTwoCommits(t *testing.T) throwawayRepo {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	commit := func(content string) string {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, dir, "add", "f.txt")
		mustGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", content)
		return mustGit(t, dir, "rev-parse", "HEAD")
	}
	r := throwawayRepo{dir: dir}
	r.first = commit("one")
	r.second = commit("two")
	r.index = mustGit(t, dir, "ls-files", "--stage")
	return r
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(dir, args...)
	if err != nil {
		t.Fatalf("git %v in %q: %v", args, dir, err)
	}
	return strings.TrimSpace(out)
}
