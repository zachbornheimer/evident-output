package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestRecordName_PlanOmitsQuantityInPlain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("plan"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	out.Task("cleanup").RecordName("delete", "foo")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	assertNamedDeleteFoo(t, buf.String(), "delete foo")
	snap := out.Snapshot()
	if len(snap.Plans) != 1 || len(snap.Plans[0].Records) != 1 {
		t.Fatalf("plans=%d", len(snap.Plans))
	}
	if snap.Plans[0].Records[0].HasQty {
		t.Fatal("HasQty must be false")
	}
}

// TestRecordName_ChangesOmitsQuantityInPlain proves the applied (Changes)
// ledger for a RecordName call: no quantity, and the applied ledger
// conjugates to past tense ("deleted") exactly like the named mutation
// verbs — RecordName's imperative "delete" here isn't a raw ledger
// primitive isolated from Add/Delete/etc.
func TestRecordName_ChangesOmitsQuantityInPlain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("done"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("cleanup").RecordName("delete", "foo")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	assertNamedDeleteFoo(t, buf.String(), "deleted foo")
	snap := out.Snapshot()
	if len(snap.Changes) != 1 || len(snap.Changes[0].Records) != 1 {
		t.Fatalf("changes=%d", len(snap.Changes))
	}
	if snap.Changes[0].Records[0].HasQty {
		t.Fatal("HasQty must be false")
	}
}

func assertNamedDeleteFoo(t *testing.T, s, wantPhrase string) {
	t.Helper()
	if strings.Contains(s, "delete 1 foo") || strings.Contains(s, "deleted 1 foo") {
		t.Fatalf("RecordName must omit quantity; got:\n%s", s)
	}
	collapsed := strings.Join(strings.Fields(s), " ")
	if !strings.Contains(collapsed, wantPhrase) {
		t.Fatalf("want %q (no quantity) in:\n%s", wantPhrase, s)
	}
}
