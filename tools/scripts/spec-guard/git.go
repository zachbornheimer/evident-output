package main

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	statusModified = "M"
	renameScore    = "R"
	copyScore      = "C"
)

// Change is one path touched between the base and HEAD.
type Change struct {
	Status string
	Path   string
}

// Git is the repository boundary of the guard; tests substitute a fake.
type Git interface {
	// NameStatus lists the paths changed in `base...HEAD`.
	NameStatus(base string) ([]Change, error)
	// FileDiff returns the zero-context unified diff of one path in `base...HEAD`.
	FileDiff(base, path string) (string, error)
}

type execGit struct{ root string }

func (g execGit) run(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func (g execGit) NameStatus(base string) ([]Change, error) {
	out, err := g.run("diff", "--name-status", base+"...HEAD")
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

func (g execGit) FileDiff(base, path string) (string, error) {
	return g.run("diff", "-U0", base+"...HEAD", "--", path)
}

// parseNameStatus expands renames and copies into one Change per path so a
// protected file cannot be moved out from under the guard.
func parseNameStatus(out string) []Change {
	var changes []Change
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		if strings.HasPrefix(status, renameScore) || strings.HasPrefix(status, copyScore) {
			status = status[:1]
		}
		for _, p := range fields[1:] {
			changes = append(changes, Change{Status: status, Path: p})
		}
	}
	return changes
}
