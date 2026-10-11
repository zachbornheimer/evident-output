package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// A callback that lends its slot to one nested Task after another learns what
// it had started once, not for each Task. A goroutine it starts between two
// nested Tasks moves the process's goroutine count, so the next claim must
// learn of it: the goroutine stays the callback's, and the second Task does
// not answer for it.
func TestLentSlotClaimStillLeavesTheLendersLaterGoroutineWithTheLender(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(1))
	outer, first, second := declare(g, "outer"), declare(g, "first"), declare(g, "second")
	readNow, got := make(chan struct{}), make(chan Reader)
	var reader Reader
	submitWork(g, first, settlingWork(g, first, func() error { return nil }))
	submitWork(g, second, settlingWork(g, second, func() error {
		close(readNow)
		reader = <-got
		return nil
	}))
	submitWork(g, outer, settlingWork(g, outer, func() error {
		if err := g.Wait(first); err != nil {
			return err
		}
		go func() {
			<-readNow
			got <- g.CurrentReader()
		}()
		return g.Wait(second)
	}))
	g.Kick()
	g.Drain()
	if reader.Task != outer || !reader.Spawned {
		name := "none"
		if reader.Task != nil {
			name = reader.Task.Name
		}
		t.Fatalf("a goroutine the lender started after its first nested Task read as %q (spawned %v), want %q", name, reader.Spawned, outer.Name)
	}
}
