package guards_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// repoRootMarker is the file that exists only at the repository root, so the
// guards find the root package no matter how deep their own directory sits.
const repoRootMarker = "CONTRACT.md"

// TestMain runs every guard from the repository root: they were written to
// read testdata/, docs/ and the root package's sources by relative path.
func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "guards:", err)
		os.Exit(1)
	}
	if err := os.Chdir(root); err != nil {
		fmt.Fprintf(os.Stderr, "guards: enter repository root %s: %v\n", root, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// moduleRoot returns the repository root the guards run from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// findRepoRoot returns the nearest ancestor of the working directory that
// holds repoRootMarker.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, repoRootMarker)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s above the working directory", repoRootMarker)
		}
		dir = parent
	}
}
