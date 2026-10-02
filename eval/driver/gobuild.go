package driver

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

const (
	maxSubmissionBytes = 256 << 10
	maxSubmissionFiles = 16
	workDirPattern     = "evo-eval-build-*"
	testFileSuffix     = "_test.go"
)

var candidateFileName = regexp.MustCompile(`^[A-Za-z0-9_-]+\.go$`)

// ValidateFiles rejects a submission that is empty, oversized, or names files
// outside the candidate package (paths, tests, non-Go files).
func ValidateFiles(files map[string]string) error {
	if len(files) == 0 {
		return fmt.Errorf("validate files: no files given")
	}
	if len(files) > maxSubmissionFiles {
		return fmt.Errorf("validate files: %d files exceeds the limit of %d", len(files), maxSubmissionFiles)
	}
	total := 0
	for name, body := range files {
		if !candidateFileName.MatchString(name) || strings.HasSuffix(name, testFileSuffix) {
			return fmt.Errorf("validate files: file name %q must be a plain non-test .go file name in package main", name)
		}
		total += len(body)
	}
	if total > maxSubmissionBytes {
		return fmt.Errorf("validate files: %d bytes exceeds the limit of %d", total, maxSubmissionBytes)
	}
	return nil
}

// FileSystem exposes files as a candidate tree for the grader.
func FileSystem(files map[string]string) fstest.MapFS {
	tree := fstest.MapFS{}
	for name, body := range files {
		tree[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return tree
}

// SortedNames lists file names in order, for stable transcripts.
func SortedNames(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// WorkDirMaker makes a scratch directory and the function that removes it.
type WorkDirMaker func() (dir string, cleanup func(), err error)

// TempWorkDir is the real WorkDirMaker, under the OS temp directory.
func TempWorkDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", workDirPattern)
	if err != nil {
		return "", nil, fmt.Errorf("create scratch directory: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// Builder compiles candidate files in a throwaway module against the
// library checkout and reports only what the compiler said.
type Builder struct {
	Grader  evaltask.Grader
	Task    evaltask.Task
	WorkDir WorkDirMaker
}

// Build returns the compiler output and whether the candidate compiled. It
// reveals nothing else the grader computes.
func (b Builder) Build(ctx context.Context, files map[string]string) (string, bool, error) {
	if err := ValidateFiles(files); err != nil {
		return "", false, err
	}
	dir, cleanup, err := b.WorkDir()
	if err != nil {
		return "", false, err
	}
	defer cleanup()
	report, err := b.Grader.GradeSource(ctx, b.Task, FileSystem(files), dir)
	if err != nil {
		return "", false, fmt.Errorf("build candidate for task %s: %w", b.Task.ID, err)
	}
	return report.BuildOutput, report.Compiles, nil
}
