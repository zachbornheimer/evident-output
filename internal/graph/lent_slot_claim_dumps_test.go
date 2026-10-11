package graph

import (
	"fmt"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// The documented nested shape: a callback Defines a Task and Waits on it at
// ceiling 1, so the Task runs inline in the lent slot. The nested Task starts
// one goroutine (as Exec does for its pipes). That goroutine is the nested
// Task's, not the lender's, so the lender, which started nothing between two
// nested Tasks, learns what it had started once and not again for each
// Task: the stops of the world stay few however many Tasks it waits on.
func TestLentSlotInlineClaimsTakeFewDumps(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	for range idleGoroutines {
		go func() { <-stop }()
	}
	g := New(record.NewRun(), WithMaxConcurrency(1))
	outer := declare(g, "outer")
	var dumps int64
	var waitErr error
	submitWork(g, outer, settlingWork(g, outer, func() error {
		dumps = dumpsDuring(func() {
			for i := range plainCalls {
				child := declare(g, fmt.Sprint("child", i))
				submitWork(g, child, settlingWork(g, child, func() error {
					done := make(chan struct{})
					go func() { close(done) }()
					<-done
					return nil
				}))
				if err := g.Wait(child); err != nil && waitErr == nil {
					waitErr = err
				}
			}
		})
		return nil
	}))
	g.Kick()
	if err := g.Wait(outer); err != nil {
		t.Fatal(err)
	}
	g.Drain()
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if dumps > fewDumps {
		t.Fatalf("%d lent-slot inline claims with %d idle goroutines took %d stop-the-world dumps, over %d",
			plainCalls, idleGoroutines, dumps, fewDumps)
	}
}
