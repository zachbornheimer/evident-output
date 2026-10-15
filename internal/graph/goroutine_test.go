package graph

import "testing"

// A goroutine's lineage chain runs from the one that started it outward,
// however many goroutines sit between it and the test.
func TestLineageChainRunsNearestFirst(t *testing.T) {
	parentID := make(chan GoroutineID, 1)
	ancestors := make(chan []GoroutineID, 1)
	release := make(chan struct{})
	go func() {
		parentID <- CurrentGoroutine()
		done := make(chan struct{})
		go func() {
			_, creator := CurrentGoroutineLineage()
			ancestors <- processLineage.chain(creator, nil)
			close(done)
		}()
		<-done
		<-release
	}()
	got, parent := <-ancestors, <-parentID
	close(release)

	if len(got) < 2 || got[0] != parent {
		t.Fatalf("ancestors = %v, want parent %d first, then the goroutine that started it", got, parent)
	}
	// Goroutine ids are handed out in batches per processor, so a creator's id
	// may be the larger: the chain's second link is the test, which started
	// the parent, whatever the ids.
	if self := CurrentGoroutine(); got[1] != self {
		t.Fatalf("ancestors = %v, want the test goroutine %d after the parent", got, self)
	}
}

// GODEBUG=tracebackancestors appends the creators' own tracebacks to a
// goroutine's. The creator read is still the goroutine's own: an ancestor's
// "created by" line names no goroutine, so only the first one does. The text
// is what the runtime prints (go1.24, tracebackancestors=5).
func TestGoroutineCreatorIgnoresTracebackAncestors(t *testing.T) {
	trace := []byte("goroutine 19 [running]:\n" +
		"main.main.func1.1()\n\t/work/main.go:3 +0x48\n" +
		"created by main.main.func1 in goroutine 18\n\t/work/main.go:3 +0x68\n" +
		"[originating from goroutine 18]:\n" +
		"main.main.func1(...)\n\t/work/main.go:3 +0x68\n" +
		"created by main.main\n\t/work/main.go:3 +0x74\n" +
		"[originating from goroutine 1]:\n" +
		"main.main(...)\n\t/work/main.go:3 +0x74\n")
	if self, creator := parseGoroutineHeader(trace), parseGoroutineCreator(trace); self != 19 || creator != 18 {
		t.Fatalf("self, creator = %d, %d, want 19, 18", self, creator)
	}
}

// Only the ledger's own start function marks a pooled worker. A method of the
// same name and receiver in another package starts goroutines that are not
// evo workers, and reading them as workers would end their chains early.
func TestPooledWorkerNeedsThePackageQualifiedLedgerFrame(t *testing.T) {
	const ours = "github.com/zachbornheimer/evident-output/v2/internal/graph.(*spawnLedger).start"
	for _, tc := range []struct {
		creator string
		worker  bool
	}{
		{ours, true},
		{"example.com/other/internal/graph.(*spawnLedger).start", false},
		{"example.com/other.(*spawnLedger).start", false},
		{"(*spawnLedger).start", false},
		{ours + "Other", false},
	} {
		dump := []byte("goroutine 9 [chan receive]:\nexample.com/x.run()\n\t/work/x.go:1 +0x1\n" +
			"created by " + tc.creator + " in goroutine 7\n\t/work/x.go:2 +0x2\n")
		creators, workers := goroutineOrigins(dump)
		if _, got := workers[9]; got != tc.worker {
			t.Errorf("created by %q: worker = %v, want %v", tc.creator, got, tc.worker)
		}
		if creators[9] != 7 {
			t.Errorf("created by %q: creator = %d, want 7", tc.creator, creators[9])
		}
	}
}
