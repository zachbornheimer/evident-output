package graph

import "testing"

// A goroutine's ancestors run from the one that started it outward, however
// many goroutines sit between it and the test.
func TestGoroutineAncestorsRunNearestFirst(t *testing.T) {
	parentID := make(chan GoroutineID, 1)
	ancestors := make(chan []GoroutineID, 1)
	release := make(chan struct{})
	go func() {
		parentID <- CurrentGoroutine()
		done := make(chan struct{})
		go func() {
			ancestors <- GoroutineAncestors()
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
	if got[1] >= got[0] {
		t.Fatalf("ancestors = %v, want each older than the goroutine it started", got)
	}
}
