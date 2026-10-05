// Command bisectability-check proves every first-parent commit in a range
// builds on its own — a merge sequence where an intermediate commit only
// compiles once a *later* commit in the same merge lands (increment 3's
// 48baff0 is the known instance) breaks git bisect for anyone landing
// between those two commits, silently, until a bisect run hits exactly
// that range.
//
// Usage:
//
//	go run ./tools/scripts/bisectability-check [BASE] [HEAD]
//
// BASE defaults to $BISECTABILITY_BASE, then "codex/v06-acceptance". HEAD
// defaults to "HEAD". Each commit in `git rev-list --first-parent
// BASE..HEAD` (oldest first) is checked out into its own temporary git
// worktree and built with `go build ./...`; the first commit that fails to
// build is reported and the tool exits nonzero.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bisectability-check:", err)
		os.Exit(1)
	}
}

// defaultBase is the integration branch's own well-known starting point
// (the work order that created integrate/evo-1.0 branched from it) — the
// one anchor every merge commit added on top of, so a bare `go run
// ./tools/scripts/bisectability-check` with no arguments checks exactly
// the range this tool exists to guard.
const defaultBase = "codex/v06-acceptance"

func run(args []string) error {
	base := firstNonEmpty(argAt(args, 0), os.Getenv("BISECTABILITY_BASE"), defaultBase)
	head := firstNonEmpty(argAt(args, 1), "HEAD")

	commits, err := firstParentCommits(base, head)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Printf("bisectability-check: no first-parent commits in %s..%s\n", base, head)
		return nil
	}

	worktree, cleanup, err := newScratchWorktree()
	if err != nil {
		return err
	}
	defer cleanup()

	for i, sha := range commits {
		if err := checkoutDetached(worktree, sha); err != nil {
			return err
		}
		if out, err := buildAll(worktree); err != nil {
			return fmt.Errorf("commit %d/%d %s does not build on its own:\n%s\n%w",
				i+1, len(commits), sha, out, err)
		}
		fmt.Printf("bisectability-check: %d/%d %s builds\n", i+1, len(commits), sha)
	}
	return nil
}

// firstParentCommits returns base..head's first-parent commits, oldest
// first — the exact chain `git bisect --first-parent` walks.
func firstParentCommits(base, head string) ([]string, error) {
	out, err := runGit("", "rev-list", "--first-parent", "--reverse", base+".."+head)
	if err != nil {
		return nil, fmt.Errorf("rev-list %s..%s: %w", base, head, err)
	}
	var commits []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}

// newScratchWorktree adds a detached, disposable git worktree this tool
// repeatedly re-checks-out to a different commit — one worktree reused
// across every commit, rather than one per commit, keeps a long range cheap.
func newScratchWorktree() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "evo-bisectability-")
	if err != nil {
		return "", nil, fmt.Errorf("create scratch dir: %w", err)
	}
	if _, err := runGit("", "worktree", "add", "--detach", "--force", dir, "HEAD"); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("worktree add %s: %w", dir, err)
	}
	cleanup = func() {
		_, _ = runGit("", "worktree", "remove", "--force", dir)
		_ = os.RemoveAll(dir)
	}
	return dir, cleanup, nil
}

func checkoutDetached(worktree, sha string) error {
	if _, err := runGit(worktree, "checkout", "--detach", "--force", sha); err != nil {
		return fmt.Errorf("git checkout %s in %s: %w", sha, worktree, err)
	}
	return nil
}

func buildAll(worktree string) (string, error) {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = worktree
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

func argAt(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
