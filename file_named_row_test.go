package evo_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// Named ledger rows — a verb and one named object, no quantity — come only
// from evo-native operations since 1.1 removed RecordName (ZYS-974):
// evo.File's "write <path>" is the one a test can drive without a process.

func TestFileRow_PlanOmitsQuantityInPlain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := writeFooThroughFile(t, &buf, true)
	assertNamedFileRow(t, buf.String(), "write")
	snap := out.Snapshot()
	if len(snap.Plans) != 1 || len(snap.Plans[0].Records) != 1 {
		t.Fatalf("plans=%+v", snap.Plans)
	}
	if snap.Plans[0].Records[0].HasQty {
		t.Fatal("HasQty must be false")
	}
}

// TestFileRow_ChangesOmitsQuantityInPlain proves the applied (Changes)
// ledger for a named row: no quantity, and the verb conjugates to past
// tense ("wrote") exactly like a quantified Effect's does.
func TestFileRow_ChangesOmitsQuantityInPlain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := writeFooThroughFile(t, &buf, false)
	assertNamedFileRow(t, buf.String(), "wrote")
	snap := out.Snapshot()
	if len(snap.Changes) != 1 || len(snap.Changes[0].Records) != 1 {
		t.Fatalf("changes=%+v", snap.Changes)
	}
	if snap.Changes[0].Records[0].HasQty {
		t.Fatal("HasQty must be false")
	}
}

// writeFooThroughFile runs one Task whose Define writes foo.txt through
// evo.File against an in-memory filesystem, and returns the finished run.
func writeFooThroughFile(t *testing.T, w io.Writer, dryRun bool) *evo.Output {
	t.Helper()
	dir := t.TempDir()
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: w, Stderr: io.Discard, Title: "cleanup",
		Color: evo.ColorNever, Plain: true, DryRun: dryRun,
		FileFS: testkit.NewFileFS(), StateDir: dir,
	})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("cleanup").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: filepath.Join(dir, "foo.txt"), Contents: []byte("foo")})
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertNamedFileRow(t *testing.T, s, verb string) {
	t.Helper()
	collapsed := strings.Join(strings.Fields(s), " ")
	if strings.Contains(collapsed, verb+" 1 ") {
		t.Fatalf("a named row must omit quantity; got:\n%s", s)
	}
	if !strings.Contains(collapsed, verb+" ") || !strings.Contains(collapsed, "foo.txt") {
		t.Fatalf("want %q naming foo.txt in:\n%s", verb, s)
	}
}
