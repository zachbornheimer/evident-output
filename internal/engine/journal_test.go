package engine

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// naiveJournal is the per-event rule the journal must stay equivalent to:
// append, then while over the cap drop the oldest non-critical event, or
// the oldest critical one when nothing else is left.
func naiveJournal(types []string, limit int) []uint64 {
	var kept []Event
	for i, typ := range types {
		kept = append(kept, Event{Type: typ, Sequence: uint64(i + 1)})
		for len(kept) > limit {
			drop := slices.IndexFunc(kept, func(e Event) bool { return !criticalEventType(e.Type) })
			drop = max(drop, 0)
			kept = slices.Delete(kept, drop, drop+1)
		}
	}
	return sequences(kept)
}

func sequences(events []Event) []uint64 {
	out := make([]uint64, len(events))
	for i, e := range events {
		out[i] = e.Sequence
	}
	return out
}

// TestJournalRetainsWhatPerEventCompactionWould proves batching changed
// the cost of compaction, not what the journal retains.
func TestJournalRetainsWhatPerEventCompactionWould(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 200 {
		limit := 1 + rng.IntN(20)
		n := rng.IntN(120)
		criticalShare := rng.Float64()
		types := make([]string, n)
		for i := range types {
			types[i] = "task.done"
			if rng.Float64() < criticalShare {
				types[i] = "task.failed"
			}
		}
		var j journal
		for _, typ := range types {
			j.append(Event{Type: typ}, limit)
		}
		got := sequences(j.snapshot(limit))
		want := naiveJournal(types, limit)
		if !slices.Equal(got, want) {
			t.Fatalf("trial %d (limit %d): retained %v, want %v", trial, limit, got, want)
		}
	}
}

// TestJournalAppendIsAmortizedConstant guards the hot path: past the cap,
// each event used to scan and shift the whole journal under o.mu. Counting
// compaction passes instead of timing keeps load from flaking it.
func TestJournalAppendIsAmortizedConstant(t *testing.T) {
	const limit = 1000
	var j journal
	moved := 0
	for range 20 * limit {
		before := len(j.events)
		j.append(Event{Type: "task.done"}, limit)
		if len(j.events) <= before {
			moved += before + 1
		}
	}
	if moved > 2*20*limit {
		t.Fatalf("compaction touched %d events for %d appends; want amortized O(1) per event", moved, 20*limit)
	}
	if got := len(j.snapshot(limit)); got != limit {
		t.Fatalf("snapshot holds %d events, want the cap %d", got, limit)
	}
}
