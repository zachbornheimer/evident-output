package evo

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// fileParentRun runs one Task whose Define writes out/a.html through File on
// the real filesystem, from a fresh workspace, and returns the Plain output.
func fileParentRun(t *testing.T, dir string, dryRun bool) (string, error) {
	t.Helper()
	t.Chdir(dir)
	var buf bytes.Buffer
	out := initRun(Config{Name: "site", DryRun: dryRun, Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	out.Task("write page").Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: "out/a.html", Contents: []byte("<p>hi</p>")})
	})
	err := out.finish()
	return buf.String(), err
}

// TestFileCreatesParentAndFailsInOneClause pins that File states the desired
// state of one file: a missing parent directory is created (never created by
// a dry run, which only plans it), and the failure that remains reads as one
// clause naming the workspace-relative path.
func TestFileCreatesParentAndFailsInOneClause(t *testing.T) {
	t.Run("creates the missing parent directory", func(t *testing.T) {
		dir := t.TempDir()
		got, _ := fileParentRun(t, dir, false)
		contents, readErr := os.ReadFile(filepath.Join(dir, "out", "a.html"))
		if readErr != nil || string(contents) != "<p>hi</p>" {
			t.Fatalf("out/a.html = %q, %v; want the written page\n%s", contents, readErr, got)
		}
		info, statErr := os.Stat(filepath.Join(dir, "out"))
		if statErr != nil || !info.IsDir() {
			t.Fatalf("out is not a directory: %v", statErr)
		}
	})

	t.Run("dry run plans the write and creates nothing", func(t *testing.T) {
		dir := t.TempDir()
		got, _ := fileParentRun(t, dir, true)
		if _, statErr := os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(statErr) {
			t.Fatalf("dry run created out/: %v", statErr)
		}
		if !bytes.Contains([]byte(got), []byte("write out/a.html")) {
			t.Fatalf("dry run must plan %q; got:\n%s", "write out/a.html", got)
		}
	})

	t.Run("failure is one clause with the workspace-relative path", func(t *testing.T) {
		dir := t.TempDir()
		if os.Geteuid() == 0 {
			t.Skip("root writes into a read-only directory")
		}
		out := filepath.Join(dir, "out")
		if err := os.Mkdir(out, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(out, 0o700) })
		got, _ := fileParentRun(t, dir, false)
		want := "✗ write page  out/a.html: permission denied\n" +
			"\n[failed]  site\n"
		if got != want {
			t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
		}
	})
}
