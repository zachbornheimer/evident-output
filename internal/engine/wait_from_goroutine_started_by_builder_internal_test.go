package engine

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// A builder that starts a goroutine and blocks on it (the errgroup shape)
// hands its Wait to a goroutine whose own stack shows no builder. The Wait is
// refused all the same, at every ceiling: left parked, it would hold Finish
// forever behind the builder it is waiting out.
func TestSlice36_WaitFromAGoroutineABuilderStartedIsRefused(t *testing.T) {
	for _, ceiling := range []int{1, 4} {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			out := quietOutput(Config{MaxConcurrency: ceiling})
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
		})
	}
}
