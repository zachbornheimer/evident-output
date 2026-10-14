package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	goTestTimeout = "5m"
	goTestPackage = "2"
	filePerm      = 0o644
	dirPerm       = 0o755
	// git merge-file exits with the conflict count, capped at 127.
	maxMergeConflicts = 127
)

type scratchSandbox struct{}

// UndoFix reverts each file with `git merge-file`: the scratch copy is the
// current side, the fixed blob the base, the pre-fix blob the other side.
func (s scratchSandbox) UndoFix(dir string, changes []FileChange) error {
	for _, c := range changes {
		if !filepath.IsLocal(c.Path) {
			return fmt.Errorf("non-local path %q", c.Path)
		}
		err := s.undo(filepath.Join(dir, filepath.FromSlash(c.Path)), c)
		var conflict *ConflictError
		if errors.As(err, &conflict) {
			conflict.Path = c.Path
			return conflict
		}
		if err != nil {
			return fmt.Errorf("%s: %w", c.Path, err)
		}
	}
	return nil
}

func (s scratchSandbox) undo(target string, c FileChange) error {
	switch c.Kind {
	case Added:
		return removeIfUnchanged(target, c.Fixed)
	case Deleted:
		if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
			return err
		}
		return os.WriteFile(target, c.Before, filePerm)
	}
	return mergeBack(target, c)
}

// removeIfUnchanged deletes a file the fix created; a later edit to it is a
// conflict the tool cannot resolve.
func removeIfUnchanged(target string, fixed []byte) error {
	current, err := os.ReadFile(target)
	if err != nil {
		return &ConflictError{Path: target, Reason: "file the fix added is gone"}
	}
	if !bytes.Equal(current, fixed) {
		return &ConflictError{Path: target, Reason: "file the fix added was edited after the fix"}
	}
	return os.Remove(target)
}

func mergeBack(target string, c FileChange) error {
	work, err := os.MkdirTemp("", "red-proof-merge-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	fixed, before := filepath.Join(work, "fixed"), filepath.Join(work, "before")
	if err := os.WriteFile(fixed, c.Fixed, filePerm); err != nil {
		return err
	}
	if err := os.WriteFile(before, c.Before, filePerm); err != nil {
		return err
	}
	out, err := exec.Command("git", "merge-file", target, fixed, before).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() <= maxMergeConflicts {
		return &ConflictError{Path: target, Reason: fmt.Sprintf("%d conflicting hunks", exit.ExitCode())}
	}
	if err != nil {
		return fmt.Errorf("git merge-file: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (scratchSandbox) RunTests(dir, pkg string, names []string) ([]byte, error) {
	run := "^(" + strings.Join(names, "|") + ")$"
	cmd := exec.Command("go", "test", "-json", "-p", goTestPackage, "-count=1", "-timeout", goTestTimeout, "-run", run, pkg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return nil, fmt.Errorf("go test %s: %w", pkg, err)
	}
	return out, nil
}

func tempScratch() (string, func(), error) {
	dir, err := os.MkdirTemp("", "red-proof-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp directory: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
