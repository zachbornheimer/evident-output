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
