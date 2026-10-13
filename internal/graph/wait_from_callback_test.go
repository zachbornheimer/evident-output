package graph

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

const (
	// chainedWaitCount is how many in-callback Waits the cost test chains.
	chainedWaitCount = 200
	// idleGoroutineCount is how many parked goroutines the loaded run keeps
	// alive in the process.
	idleGoroutineCount = 5000
	// builderStillDeclaring is how long a builder keeps declaring after its
	// child's callback started, so the callback's Wait overlaps the builder.
	builderStillDeclaring = 200 * time.Millisecond
)

// chainedInCallbackWaits runs n Tasks whose callbacks each Wait, inside
// themselves, on the previous Task while idle goroutines are alive in the
// process, and returns the stops of the world the run took.
func chainedInCallbackWaits(t *testing.T, n, idle int) int64 {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	for range idle {
		go func() { <-stop }()
	}
	g := New(record.NewRun(), WithMaxConcurrency(8))
	var prev *Task
	dumps := dumpsDuring(func() {
		for i := range n {
			task := declare(g, fmt.Sprint("t", i))
			before := prev
			submitWork(g, task, settlingWork(g, task, func() error {
				if before != nil {
					if err := g.Wait(before); err != nil {
						return err
					}
				}
				time.Sleep(200 * time.Microsecond)
				return nil
			}))
			prev = task
		}
		g.Drain()
	})
	if got := prev.Rec.State(); got != record.Done {
		t.Fatalf("last chained Task = %s, want done", got)
	}
	return dumps
}

// An in-callback Wait reads its callback from its own stack, so it must not
// pay for a stop-the-world dump of every goroutine in the process: its cost
// is independent of how many goroutines are alive.
func TestInCallbackWaitCostIsIndependentOfGoroutineCount(t *testing.T) {
	loaded := chainedInCallbackWaits(t, chainedWaitCount, idleGoroutineCount)
	if loaded > fewDumps {
		t.Fatalf("%d in-callback Waits with %d idle goroutines took %d goroutine dumps, want at most %d: each Wait stops the world",
			chainedWaitCount, idleGoroutineCount, loaded, fewDumps)
	}
}

// waitFromBuilderChild settles a Task, runs a container builder that declares
// a child whose callback calls wait, keeps the builder declaring while the
// child runs, and returns what the child's Wait on the settled Task got.
func waitFromBuilderChild(t *testing.T, wait func(g *Graph, settled *Task, got chan<- error)) error {
	t.Helper()
	g := New(record.NewRun(), WithMaxConcurrency(4))
	settled := declare(g, "settled")
	submitWork(g, settled, settlingWork(g, settled, func() error { return nil }))
	if err := g.Wait(settled); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	group := g.AddContainer(nil, "builder", false)
	gate := g.AddGate(group, func() error {
		child := must(g.AddTask(group, "child", record.TaskInit{State: record.Pending}))
		submitWork(g, child, settlingWork(g, child, func() error {
			wait(g, settled, got)
			return nil
		}))
		g.Kick()
		time.Sleep(builderStillDeclaring)
		return nil
	})
	g.Submit(gate, Work{})
	g.Drain()
	select {
	case err := <-got:
		return err
	default:
		t.Fatal("the builder's child never waited")
		return nil
	}
}

// A builder child's callback runs on a worker the builder's goroutine kicked.
// Its stack names the callback it runs, so a Wait on its own goroutine is the
// callback's, not the builder's.
func TestWaitFromABuilderChildCallbackIsNotInsideTheBuilder(t *testing.T) {
	err := waitFromBuilderChild(t, func(g *Graph, settled *Task, got chan<- error) { got <- g.Wait(settled) })
	if err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
}

// A goroutine a builder child's callback started answers to that callback,
// not to the builder above it: the ancestor walk stops at the callback.
func TestWaitFromAGoroutineABuilderChildCallbackStartedIsNotInsideTheBuilder(t *testing.T) {
	err := waitFromBuilderChild(t, func(g *Graph, settled *Task, got chan<- error) {
		done := make(chan struct{})
		go func() { got <- g.Wait(settled); close(done) }()
		<-done
	})
	if errors.Is(err, ErrWaitInBuilder) {
		t.Fatalf("Wait from a goroutine the child's callback started = %v: refused as inside the builder", err)
	}
	if err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
}
