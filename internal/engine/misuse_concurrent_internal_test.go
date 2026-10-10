package engine

import (
	"errors"
	"sync"
	"testing"
)

// TestMisuseRecordedFromGraphGoroutinesNeedsNoOutputLock proves the misuse
// sink the graph calls is safe from any goroutine without Output.mu: the
// graph's own workers report a wait deadlock or a dependency cycle there.
// Run under -race, an unguarded first-misuse field fails it.
func TestMisuseRecordedFromGraphGoroutinesNeedsNoOutputLock(t *testing.T) {
	o := newListenerTestOutput(t)
	errFirst := errors.New("first")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			misuseSink{o: o}.RecordMisuseFor("task", errFirst)
		})
	}
	wg.Wait()

	if got := o.firstMisuse(); !errors.Is(got, errFirst) {
		t.Errorf("first misuse = %v, want %v", got, errFirst)
	}
}
