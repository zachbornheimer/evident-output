package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseArgsFlagEitherSideOfPositional proves --output/-o works both
// before and after the repo path, per the work order's CLI contract.
func TestParseArgsFlagEitherSideOfPositional(t *testing.T) {
	cases := [][]string{
		{"--output", "out.md", "repo"},
		{"repo", "--output", "out.md"},
		{"-o", "out.md", "repo"},
		{"repo", "-o", "out.md"},
	}
	for _, args := range cases {
		got, err := parseArgs(args)
		if err != nil {
			t.Errorf("parseArgs(%v): %v", args, err)
			continue
		}
		if got.repoPath != "repo" || got.outputPath != "out.md" {
			t.Errorf("parseArgs(%v) = %+v, want repoPath=repo outputPath=out.md", args, got)
		}
	}
}

// TestRunZeroPositionalArgsFails proves a missing repo path exits with a
// usage message rather than a silent success.
func TestRunZeroPositionalArgsFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("run(nil): want an error for zero positional args, got nil")
	}
	if !strings.Contains(err.Error(), "got 0") || !strings.Contains(err.Error(), "Usage:") {
		t.Errorf("run(nil) error = %q, want a usage message naming 0 args", err)
	}
}

// TestRunTwoPositionalArgsFails proves more than one repo path exits with
// a usage message.
func TestRunTwoPositionalArgsFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"repo-a", "repo-b"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run([repo-a repo-b]): want an error for two positional args, got nil")
	}
	if !strings.Contains(err.Error(), "got 2") || !strings.Contains(err.Error(), "Usage:") {
		t.Errorf("run([repo-a repo-b]) error = %q, want a usage message naming 2 args", err)
	}
}

// TestRunUnreadablePathFails proves a nonexistent repo path fails with the
// path named in the error, not a generic message.
func TestRunUnreadablePathFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	err := run([]string{missing}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run([missing]): want an error for an unreadable path, got nil")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("run([missing]) error = %q, want it to name %q", err, missing)
	}
}

// TestRunNoGoFilesFails proves an empty (or evo-free) directory fails
// rather than silently emitting an empty document.
func TestRunNoGoFilesFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	empty := t.TempDir()
	err := run([]string{empty}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run([empty dir]): want an error, got nil")
	}
	if !strings.Contains(err.Error(), "no Go files with evo usage") {
		t.Errorf("run([empty dir]) error = %q, want it to say no evo usage was found", err)
	}
}

// TestRunHelpExitsCleanly proves --help prints usage and succeeds (exit 0
// territory), rather than being treated as a usage error.
func TestRunHelpExitsCleanly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run([--help]): %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: evo-usage-audit") {
		t.Errorf("run([--help]) stdout = %q, want usage text", stdout.String())
	}
}
