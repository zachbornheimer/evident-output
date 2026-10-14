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

// facade.go is the tool's only system boundary: every subprocess it starts and
// every file it reads goes through the functions below, each with a constant
// command name and operands checked before they reach the OS.

const (
	gitCommand = "git"
	tarCommand = "tar"
	goCommand  = "go"
)

// cleanPath rejects an empty or NUL-bearing path and returns it lexically cleaned.
func cleanPath(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("invalid path %q", path)
	}
	return filepath.Clean(path), nil
}

// readFile reads a file by its cleaned path.
func readFile(path string) ([]byte, error) {
	clean, err := cleanPath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(clean)
}

// gitOutput runs `git -C root args...` and returns its stdout; a failure
// carries git's stderr.
func gitOutput(root string, args ...string) ([]byte, error) {
	dir, err := cleanPath(root)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(gitCommand, append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

// gitArchiveInto extracts the tree at rev of the repository at root into dir.
func gitArchiveInto(root, rev, dir string) error {
	if err := checkRev(rev); err != nil {
		return err
	}
	repo, err := cleanPath(root)
	if err != nil {
		return err
	}
	into, err := cleanPath(dir)
	if err != nil {
		return err
	}
	archive := exec.Command(gitCommand, "-C", repo, "archive", "--format=tar", rev)
	extract := exec.Command(tarCommand, "-xf", "-", "-C", into)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe git archive: %w", err)
	}
	extract.Stdin = pipe
	var stderr bytes.Buffer
	archive.Stderr, extract.Stderr = &stderr, &stderr
	if err := extract.Start(); err != nil {
		return fmt.Errorf("start tar: %w", err)
	}
	if err := archive.Run(); err != nil {
		_ = extract.Wait()
		return fmt.Errorf("git archive %s: %w: %s", rev, err, stderr.String())
	}
	if err := extract.Wait(); err != nil {
		return fmt.Errorf("extract archive into %s: %w: %s", into, err, stderr.String())
	}
	return nil
}

// gitMergeFile three-way merges into target. A process that ran and exited
// non-zero reports its status with a nil error; err means git did not run.
func gitMergeFile(target, fixed, before string) (status int, output []byte, err error) {
	paths := make([]string, 0, 3)
	for _, path := range []string{target, fixed, before} {
		clean, err := cleanPath(path)
		if err != nil {
			return 0, nil, err
		}
		paths = append(paths, clean)
	}
	out, err := exec.Command(gitCommand, append([]string{"merge-file"}, paths...)...).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), out, nil
	}
	return 0, out, err
}

// gitApply applies patch to the tree in dir, all hunks or none. dir is not a
// repository, so git works on the files alone.
func gitApply(dir string, patch []byte) error {
	work, err := cleanPath(dir)
	if err != nil {
		return err
	}
	cmd := exec.Command(gitCommand, "apply", "--whitespace=nowarn", "-")
	cmd.Dir = work
	cmd.Stdin = bytes.NewReader(patch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// goTestJSON runs the named top-level tests of pkg in dir with -json. Failing
// tests are the expected output, so a non-zero exit is not an error here.
func goTestJSON(dir, pkg string, names []string) ([]byte, error) {
	work, err := cleanPath(dir)
	if err != nil {
		return nil, err
	}
	if !pkgPattern.MatchString(pkg) {
		return nil, fmt.Errorf("package %q is not a relative package path", pkg)
	}
	for _, name := range names {
		if !testPattern.MatchString(name) {
			return nil, fmt.Errorf("test name %q is not a top-level test function", name)
		}
	}
	run := "^(" + strings.Join(names, "|") + ")$"
	cmd := exec.Command(goCommand, "test", "-json", "-p", goTestPackage, "-count=1", "-timeout", goTestTimeout, "-run", run, pkg)
	cmd.Dir = work
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return nil, fmt.Errorf("go test %s: %w", pkg, err)
	}
	return out, nil
}
