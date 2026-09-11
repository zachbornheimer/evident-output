package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestLedger_EachChildEffectsRollUpToTheirCollection is the red-first proof
// for the canary's 39-line `[planned]` ledger: a 33-branch cleanup printed
// `[planned] tmp/ws-1  delete 1 local tip` thirty-three times, one row per
// item name, unbounded. The dialect's own Recommended UI for that run is
// three rows, one per subject — `[planned] branches  delete 8 local tips`.
//
// An Each child is one item of a collection, not a subject of its own; the
// collection owns the ledger line. Attributing the effect to its owner is
// also what lets the existing identical-record merge do the tally.
func TestLedger_EachChildEffectsRollUpToTheirCollection(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		Title: "zq", Subject: "repo  /tmp/flight", Preview: true, Stdout: &buf,
	})

	branches := out.Group("branches")
	tips := make([]string, 33)
	for i := range tips {
		tips[i] = fmt.Sprintf("feat/old-%02d", i)
	}
	for _, task := range branches.Each(tips) {
		task.Delete("local tip", func() error { return nil })
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := buf.String()
	if n := strings.Count(got, "[planned]"); n != 1 {
		t.Fatalf("want one planned row for the collection, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "[planned] branches") {
		t.Fatalf("the plan belongs to the affected subject:\n%s", got)
	}
	if !strings.Contains(got, "delete 33 local tips") {
		t.Fatalf("want the verb/object tally:\n%s", got)
	}
	if strings.Contains(got, "feat/old-00") {
		t.Fatalf("an Each item name is not a ledger subject:\n%s", got)
	}
}

// TestLedger_ExplicitTaskKeepsItsOwnSubject guards the other half of the
// rule: an explicitly declared task is semantically named work and keeps its
// own ledger line, whether or not it sits inside a collection.
func TestLedger_ExplicitTaskKeepsItsOwnSubject(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		Title: "zq", Subject: "repo  /tmp/flight", Preview: true, Stdout: &buf,
	})

	cleanup := out.Group("cleanup")
	cleanup.Task("remote-tracking").Delete("stale origin/*", func() error { return nil }, evo.Affected(2))
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, "[planned] remote-tracking") {
		t.Fatalf("an explicitly declared task keeps its own subject line:\n%s", got)
	}
}

// TestLedger_EachChildNamedRecordsBoundUnderTheCollection proves the named
// path the dialect points callers at when they do want item names: they roll
// up under the collection's subject and take the bounded viewport with its
// `… +N more` overflow, instead of one unbounded row per child.
func TestLedger_EachChildNamedRecordsBoundUnderTheCollection(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		Title: "zq", Subject: "repo  /tmp/flight", Preview: true, Stdout: &buf,
	})

	branches := out.Group("branches")
	tips := make([]string, 30)
	for i := range tips {
		tips[i] = fmt.Sprintf("feat/old-%02d", i)
	}
	for name, task := range branches.Each(tips) {
		task.RecordName("delete", name)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[planned]  branches") {
		t.Fatalf("named rows belong to the collection:\n%s", got)
	}
	if !strings.Contains(got, "more (not shown)") {
		t.Fatalf("want the bounded overflow line:\n%s", got)
	}
}
