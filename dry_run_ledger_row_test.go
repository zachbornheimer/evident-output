package evo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDryRunLedgerRowAlignsAndNamesTheChange pins CONTRACT.md section 3's
// planned ledger: the row streamed for a root Task shares the subject column
// of the folded section Finish writes (the 12 columns of "clean caches", not
// the wider subjects it folded away), and a File whose contents already
// match and whose mode differs plans "set mode of <path>", not a write.
func TestDryRunLedgerRowAlignsAndNamesTheChange(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	hook := "hook.sh"
	if err := os.WriteFile(filepath.Join(dir, hook), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	out := initRun(Config{Name: "probe", DryRun: true, Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	clean := out.Group("clean caches")
	for _, manager := range []string{"npm", "pip"} {
		group := clean.Group(manager)
		for _, entry := range []string{"a", "b"} {
			group.Task(entry).Define(effectOf(Delete, "cache entry", 1))
		}
	}
	out.Task("hook").After(clean).Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: hook, Contents: []byte("#!/bin/sh\n"), Mode: 0o755})
	})
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	const want = "[planned] hook          set mode of hook.sh\n" +
		"\n" +
		"[planned] clean caches  delete 4 cache entries\n"
	if got := buf.String(); !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("dry-run ledger mismatch:\n--- want (contains) ---\n%s\n--- got ---\n%s", want, got)
	}
}
