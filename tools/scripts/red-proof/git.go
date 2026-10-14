package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var hexCommit = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// checkRev accepts HEAD or a hex commit id, so no ledger text reaches git as an option or expression.
func checkRev(rev string) error {
	if rev != headRev && !hexCommit.MatchString(rev) {
		return fmt.Errorf("revision %q is neither HEAD nor a hex commit id", rev)
	}
	return nil
}

// fixPathspec limits a fix to Go source other than tests: test files stay as
// shipped, and docs and fixtures are inputs to the tests, not the code under test.
var fixPathspec = []string{"--", "*.go", ":(exclude)*_test.go"}

type repoGit struct{ root string }

func (g repoGit) Export(rev, dir string) error {
	return gitArchiveInto(g.root, rev, dir)
}

func (g repoGit) FixChanges(commit, rev string) ([]FileChange, error) {
	if err := checkRev(commit); err != nil {
		return nil, err
	}
	if err := checkRev(rev); err != nil {
		return nil, err
	}
	args := append([]string{"diff", "--name-status", "-M", commit + "^", commit}, fixPathspec...)
	status, err := g.git(args...)
	if err != nil {
		return nil, err
	}
	if moved := movedPaths(string(status)); len(moved) > 0 {
		return nil, fmt.Errorf("commit renames or copies files, which this tool does not reverse: %s", strings.Join(moved, ", "))
	}
	rewrite, err := g.modulePathRewrite(commit, rev)
	if err != nil {
		return nil, err
	}
	var changes []FileChange
	for line := range strings.SplitSeq(strings.TrimSpace(string(status)), "\n") {
		kind, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		change, err := g.change(commit, ChangeKind(kind), path)
		if err != nil {
			return nil, err
		}
		change.Fixed, change.Before = rewrite(change.Fixed), rewrite(change.Before)
		changes = append(changes, change)
	}
	return changes, nil
}

// modulePathRewrite returns a function that rewrites the commit's import paths
// to the target tree's module path, so a later module-path move does not make every
// import line a merge conflict. It is the identity when the path never moved.
func (g repoGit) modulePathRewrite(commit, rev string) (func([]byte) []byte, error) {
	then, err := g.modulePath(commit)
	if err != nil {
		return nil, err
	}
	now, err := g.modulePath(rev)
	if err != nil {
		return nil, err
	}
	if then == now {
		return func(b []byte) []byte { return b }, nil
	}
	return func(b []byte) []byte {
		return bytes.ReplaceAll(b, []byte(`"`+then), []byte(`"`+now))
	}, nil
}

func (g repoGit) modulePath(rev string) (string, error) {
	mod, err := g.git("cat-file", "blob", rev+":go.mod")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(mod), "\n") {
		if path, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(path), nil
		}
	}
	return "", fmt.Errorf("go.mod at %s has no module line", rev)
}

func (g repoGit) change(commit string, kind ChangeKind, path string) (FileChange, error) {
	if !filepath.IsLocal(path) {
		return FileChange{}, fmt.Errorf("commit %s touches non-local path %q", commit, path)
	}
	change := FileChange{Path: path, Kind: kind}
	var err error
	switch kind {
	case Added:
		change.Fixed, err = g.git("cat-file", "blob", commit+":"+path)
	case Deleted:
		change.Before, err = g.git("cat-file", "blob", commit+"^:"+path)
	case Modified:
		if change.Fixed, err = g.git("cat-file", "blob", commit+":"+path); err == nil {
			change.Before, err = g.git("cat-file", "blob", commit+"^:"+path)
		}
	default:
		err = fmt.Errorf("commit %s: unsupported change %q on %s", commit, kind, path)
	}
	return change, err
}

func (g repoGit) git(args ...string) ([]byte, error) { return gitOutput(g.root, args...) }

// movedPaths lists the paths of rename and copy rows in `git diff --name-status`.
func movedPaths(nameStatus string) []string {
	var moved []string
	for line := range strings.SplitSeq(nameStatus, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) > 1 && (strings.HasPrefix(fields[0], "R") || strings.HasPrefix(fields[0], "C")) {
			moved = append(moved, strings.Join(fields[1:], " -> "))
		}
	}
	return moved
}
