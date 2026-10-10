package engine

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// A builder that starts a goroutine and blocks on it (the errgroup shape)
// hands its Wait to a goroutine whose own stack shows no builder. The Wait is
// refused all the same: left parked, it would hold Finish forever behind the
// builder it is waiting out. A ceiling of one keeps the child from running
// ahead of the Wait, so the Wait always has to park.
func TestSlice36_WaitFromAGoroutineABuilderStartedIsRefused(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 1})
	t.Cleanup(func() { _ = out.Close() })
	var waitErr error
	out.Group("builder").Define(func(g *GroupHandle) {
		child := g.Task("child").Define(noop)
		var started sync.WaitGroup
		started.Go(func() { ; waitErr = child.Wait() })
		started.Wait()
	})

	if _, finished := within(5*time.Second, out.Finish); !finished {
		t.Fatalf("Finish hung: the Wait is neither refused nor released (waits=%d executing=%d)", out.graph.Waits(), out.graph.Executing())
	}
	if !errors.Is(waitErr, graph.ErrWaitInBuilder) {
		t.Errorf("Wait from the builder's goroutine = %v, want ErrWaitInBuilder", waitErr)
	}
}
