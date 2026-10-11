package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// A goroutine the first nested Task started and left running is not the
// second nested Task's: the second Task did not start it. f8b803cd reuses
// the lender's known set from the first Task's predating set, which omits
// the goroutines the first Task started, so the second Task's frame claims
// them.
func TestGoroutineAFinishedNestedTaskLeftRunningIsNotTheNextNestedTasks(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(1))
	outer, first, second := declare(g, "outer"), declare(g, "first"), declare(g, "second")
	readNow, got := make(chan struct{}), make(chan Reader)
	var reader Reader
	submitWork(g, first, settlingWork(g, first, func() error {
		go func() {
			<-readNow
			got <- g.CurrentReader()
		}()
		return nil
	}))
	submitWork(g, second, settlingWork(g, second, func() error {
		close(readNow)
		reader = <-got
		return nil
	}))
	submitWork(g, outer, settlingWork(g, outer, func() error {
		if err := g.Wait(first); err != nil {
			return err
		}
		return g.Wait(second)
	}))
	g.Kick()
	g.Drain()
	if reader.Task == second {
		t.Fatalf("a goroutine %q started and left running read as %q, which did not start it", first.Name, second.Name)
	}
}
