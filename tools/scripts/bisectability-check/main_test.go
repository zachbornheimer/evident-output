package main

import "testing"

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
	commits, err := firstParentCommits("733d833", "48baff0")
	if err != nil {
		t.Fatalf("firstParentCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("firstParentCommits(733d833, 48baff0) = %v, want exactly the one known-broken commit", commits)
	}

	worktree, cleanup, err := newScratchWorktree()
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
