package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanPathRejectsEmptyAndNul(t *testing.T) {
	for _, bad := range []string{"", "a\x00b"} {
		if _, err := cleanPath(bad); err == nil {
			t.Errorf("cleanPath(%q) accepted an invalid path", bad)
		}
	}
	got, err := cleanPath("a/../b//c")
	if err != nil || got != filepath.Join("b", "c") {
		t.Errorf("cleanPath = %q, %v; want b/c", got, err)
	}
}

func TestGoTestJSONRejectsUnvalidatedOperands(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]struct {
		pkg   string
		names []string
	}{
		"option as package":  {"-exec=evil", []string{"TestX"}},
		"absolute package":   {"/etc", []string{"TestX"}},
		"regex in test name": {".", []string{"TestX|.*"}},
	}
	for label, c := range cases {
		if _, err := goTestJSON(dir, c.pkg, c.names); err == nil {
			t.Errorf("%s: goTestJSON accepted pkg=%q names=%q", label, c.pkg, c.names)
		}
	}
}

func TestGitArchiveIntoRejectsNonCommitRevision(t *testing.T) {
	err := gitArchiveInto(t.TempDir(), "--output=x", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "neither HEAD nor a hex commit id") {
		t.Fatalf("got %v, want the revision check to refuse an option", err)
	}
}

func TestGitMergeFileReportsConflictCountAsStatus(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), filePerm); err != nil {
			t.Fatal(err)
		}
		return path
	}
	target, fixed, before := write("t", "mine\n"), write("f", "base\n"), write("b", "theirs\n")
	status, _, err := gitMergeFile(target, fixed, before)
	if err != nil || status != 1 {
		t.Fatalf("status=%d err=%v; want one conflicting hunk and no error", status, err)
	}
}

func TestReadFileMissingIsAnOSError(t *testing.T) {
	_, err := readFile(filepath.Join(t.TempDir(), "absent"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want os.ErrNotExist", err)
	}
}
