// Command bisectability-check proves every first-parent commit in a range
// builds on its own — a merge sequence where an intermediate commit only
// compiles once a *later* commit in the same merge lands (increment 3's
// 48baff0 is the known instance) breaks git bisect for anyone landing
// between those two commits, silently, until a bisect run hits exactly
// that range.
//
// Usage:
//
//	go run ./tools/scripts/bisectability-check [--test] [BASE] [HEAD]
//
// BASE defaults to $BISECTABILITY_BASE, then "codex/v06-acceptance". HEAD
// defaults to "HEAD". Each commit in `git rev-list --first-parent
// BASE..HEAD` (oldest first) is checked out into its own temporary git
// worktree and built with `go build ./...`; the first commit that fails to
// build is reported and the tool exits nonzero.
//
// With --test, each commit also runs `go test` for the packages it touched
// (`./...` when it changed a root file), so a red test cannot land in a
// commit apart from its fix.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/gitenv"
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

// options are the parsed command line; empty base and head take their defaults.
type options struct {
	base, head string
	runTests   bool
}

// parseOptions parses the command line. flag.ErrHelp is returned for --help,
// after the usage text is written to usage.
func parseOptions(args []string, usage io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("bisectability-check", flag.ContinueOnError)
	flags.SetOutput(usage)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(usage, "usage: bisectability-check [--test] [BASE] [HEAD]")
		flags.PrintDefaults()
	}
	flags.BoolVar(&opts.runTests, "test", false,
		"also run go test for the packages each commit touched (./... when a root file changed)")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	opts.base, opts.head = argAt(flags.Args(), 0), argAt(flags.Args(), 1)
	return opts, nil
}

func run(args []string) error {
	opts, err := parseOptions(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	base := firstNonEmpty(opts.base, os.Getenv("BISECTABILITY_BASE"), defaultBase)
	head := firstNonEmpty(opts.head, "HEAD")

	commits, err := firstParentCommits(".", base, head)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Printf("bisectability-check: no first-parent commits in %s..%s\n", base, head)
		return nil
	}

	worktree, cleanup, err := newScratchWorktree(".")
	if err != nil {
		return err
	}
	defer cleanup()

	for i, sha := range commits {
		if err := checkoutDetached(worktree, sha); err != nil {
			return err
		}
		if err := verifyCommit(worktree, sha, opts.runTests); err != nil {
			return fmt.Errorf("commit %d/%d %s: %w", i+1, len(commits), sha, err)
		}
		fmt.Printf("bisectability-check: %d/%d %s %s\n", i+1, len(commits), sha, passedSummary(opts.runTests))
	}
	return nil
}

func passedSummary(runTests bool) string {
	if runTests {
		return "builds and passes its tests"
	}
	return "builds"
}

// verifyCommit proves the commit checked out in worktree builds and, with
// runTests, that the tests of the packages it touched pass: a red test must
// land with its fix, or `git bisect run go test` lands on it.
func verifyCommit(worktree, sha string, runTests bool) error {
	if out, err := buildAll(worktree); err != nil {
		return fmt.Errorf("does not build on its own:\n%s\n%w", out, err)
	}
	if !runTests {
		return nil
	}
	packages, err := commitTestPackages(worktree, sha)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return nil
	}
	if out, err := testPackages(worktree, packages); err != nil {
		return fmt.Errorf("builds but its tests (%s) fail on their own:\n%s\n%w",
			strings.Join(packages, " "), out, err)
	}
	return nil
}

// firstParentCommits returns base..head's first-parent commits, oldest
// first — the exact chain `git bisect --first-parent` walks. repo names the
// repository explicitly; nothing is taken from the environment.
func firstParentCommits(repo, base, head string) ([]string, error) {
	out, err := runGit(repo, "rev-list", "--first-parent", "--reverse", base+".."+head)
	if err != nil {
		return nil, fmt.Errorf("rev-list %s..%s: %w", base, head, err)
	}
	var commits []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}

// newScratchWorktree adds a detached, disposable git worktree this tool
// repeatedly re-checks-out to a different commit — one worktree reused
// across every commit, rather than one per commit, keeps a long range cheap.
// The worktree is added to repo.
func newScratchWorktree(repo string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "evo-bisectability-")
	if err != nil {
		return "", nil, fmt.Errorf("create scratch dir: %w", err)
	}
	if _, err := runGit(repo, "worktree", "add", "--detach", "--force", dir, "HEAD"); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("worktree add %s: %w", dir, err)
	}
	cleanup = func() {
		_, _ = runGit(repo, "worktree", "remove", "--force", dir)
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
	out, err := gitenv.Command(dir, args...).CombinedOutput()
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
