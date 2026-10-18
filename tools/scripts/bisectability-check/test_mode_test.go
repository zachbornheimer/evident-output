package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want options
	}{
		{"defaults", nil, options{}},
		{"base and head", []string{"main", "feature"}, options{base: "main", head: "feature"}},
		{"test flag", []string{"--test"}, options{runTests: true}},
		{"test flag before positionals", []string{"--test", "main", "feature"}, options{base: "main", head: "feature", runTests: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOptions(tc.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions(%v): %v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("parseOptions(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseOptions_HelpIsNotAFailure(t *testing.T) {
	var usage strings.Builder
	if _, err := parseOptions([]string{"--help"}, &usage); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseOptions(--help) error = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(usage.String(), "--test") {
		t.Errorf("usage does not describe --test:\n%s", usage.String())
	}
}

// TestTestPackagesFor proves each changed file maps to the package whose
// tests can go red because of it, and that a root file means everything.
func TestTestPackagesFor(t *testing.T) {
	goDirs := map[string]bool{"internal/graph": true, "internal/fs": true, "tools/scripts/x": true}
	hasGoFiles := func(dir string) bool { return goDirs[dir] }
	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"go file", []string{"internal/graph/run.go"}, []string{"./internal/graph"}},
		{"two files one package", []string{"internal/graph/a.go", "internal/graph/b_test.go"}, []string{"./internal/graph"}},
		{"two packages sorted", []string{"internal/graph/a.go", "internal/fs/b.go"}, []string{"./internal/fs", "./internal/graph"}},
		{"testdata fixture", []string{"internal/graph/testdata/deep/golden.txt"}, []string{"./internal/graph"}},
		{"nested package", []string{"tools/scripts/x/main.go"}, []string{"./tools/scripts/x"}},
		{"deleted package walks up to the root package", []string{"internal/gone/a.go"}, []string{"."}},
		{"docs directory without go files", []string{"docs/migration/note.md"}, []string{"."}},
		{"root go file", []string{"internal/graph/a.go", "run.go"}, []string{"./..."}},
		{"root non-go file", []string{"go.mod"}, []string{"./..."}},
		{"nothing changed", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := testPackagesFor(tc.changed, hasGoFiles); !slices.Equal(got, tc.want) {
				t.Errorf("testPackagesFor(%v) = %v, want %v", tc.changed, got, tc.want)
			}
		})
	}
}

// TestVerifyCommit_TestModeCatchesARedTest is the red-first proof for --test:
// a commit that builds but ships a failing test passes the build-only check
// and must fail the test check.
func TestVerifyCommit_TestModeCatchesARedTest(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	red := commitFiles(t, dir, "red test", map[string]string{
		"go.mod":      "module example.com/redgreen\n\ngo 1.25\n",
		"a/a.go":      "package a\n\nfunc One() int { return 1 }\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 2 {\n\t\tt.Fatal(\"One() is not 2\")\n\t}\n}\n",
	})

	worktree, cleanup, err := newScratchWorktree(dir)
	if err != nil {
		t.Fatalf("newScratchWorktree: %v", err)
	}
	t.Cleanup(cleanup)
	if err := checkoutDetached(worktree, red); err != nil {
		t.Fatalf("checkoutDetached: %v", err)
	}

	if err := verifyCommit(worktree, red, false); err != nil {
		t.Fatalf("build-only verifyCommit rejected a commit that builds: %v", err)
	}
	err = verifyCommit(worktree, red, true)
	if err == nil {
		t.Fatal("test-mode verifyCommit accepted a commit with a failing test")
	}
	if !strings.Contains(err.Error(), "One() is not 2") {
		t.Errorf("error does not carry the failing test output: %v", err)
	}
}

func TestVerifyCommit_TestModePassesAGreenCommit(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	green := commitFiles(t, dir, "green test", map[string]string{
		"go.mod":      "module example.com/redgreen\n\ngo 1.25\n",
		"a/a.go":      "package a\n\nfunc One() int { return 1 }\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal(\"One() is not 1\")\n\t}\n}\n",
	})

	worktree, cleanup, err := newScratchWorktree(dir)
	if err != nil {
		t.Fatalf("newScratchWorktree: %v", err)
	}
	t.Cleanup(cleanup)
	if err := checkoutDetached(worktree, green); err != nil {
		t.Fatalf("checkoutDetached: %v", err)
	}
	if err := verifyCommit(worktree, green, true); err != nil {
		t.Fatalf("test-mode verifyCommit rejected a green commit: %v", err)
	}
}

// commitFiles writes files under dir, commits them and returns the commit.
func commitFiles(t *testing.T, dir, message string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustGit(t, dir, "add", "-A")
	t.Setenv("HK", "0") // the developer's global hk hooks must not gate a throwaway repo
	mustGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com",
		"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", message)
	return mustGit(t, dir, "rev-parse", "HEAD")
}
