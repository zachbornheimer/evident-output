package evo_test

import (
	"errors"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestEffect_RejectsContentFree is the red-first proof that a Record with
// empty mutation semantics — no verb, no object — is misuse, not a painted
// "[changed] zq" row with nothing in it.
func TestEffect_RejectsContentFree(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Title: "zq", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("zq")
	task.Record("", 0, "")

	if !errors.Is(out.Err(), evo.ErrInvalidConfig) {
		t.Fatalf("Err() = %v, want ErrInvalidConfig for a content-free Record", out.Err())
	}
	if contentFreeChange(out.Snapshot()) {
		t.Fatalf("content-free Record must not appear in Snapshot.Changes, got %+v", out.Snapshot().Changes)
	}

	task.Done()
}

// TestRecord_MeaningfulVerbObjectStillCommits keeps the honest ledger
// spelling: Record("delete", 40, "local tips") still lands in Changes.
func TestRecord_MeaningfulVerbObjectStillCommits(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Title: "zq", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("zq")
	task.Record("delete", 40, "local tips")
	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	if len(snap.Changes) != 1 || len(snap.Changes[0].Records) != 1 {
		t.Fatalf("want one committed effect, got %+v", snap.Changes)
	}
	got := snap.Changes[0].Records[0]
	if got.Verb != "deleted" || got.Quantity != 40 || got.Object != "local tips" {
		t.Fatalf("record = %+v, want deleted 40 local tips", got)
	}
}

func contentFreeChange(snap evo.Snapshot) bool {
	for _, ch := range snap.Changes {
		if ch.Subject == "" {
			return true
		}
		if len(ch.Records) == 0 && ch.IntendedVerb == "" {
			return true
		}
		for _, rec := range ch.Records {
			if rec.Verb == "" && rec.Object == "" {
				return true
			}
		}
	}
	return false
}
