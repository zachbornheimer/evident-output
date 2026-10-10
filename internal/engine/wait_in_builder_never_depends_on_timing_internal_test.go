package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// A Wait inside a container builder is refused or answered by the declared
// order alone: never because the awaited Task happened to settle first.
func TestSlice36_WaitInABuilderIsRefusedWhetherOrNotTheAwaitedTaskSettled(t *testing.T) {
	cases := map[string]time.Duration{
		"awaited Task already settled":      0,
		"awaited Task still running":        20 * time.Millisecond,
		"awaited Task settled a moment ago": time.Millisecond,
	}
	for name, runtime := range cases {
		t.Run(name, func(t *testing.T) {
			out := quietOutput(Config{MaxConcurrency: 4})
			t.Cleanup(func() { _ = out.Close() })
			outside := out.Task("outside").Define(func(context.Context) error { time.Sleep(runtime); return nil })
			if runtime == 0 {
				_ = outside.Wait()
			}
			var waitErr error
			out.Group("builder").Define(func(g *GroupHandle) {
				waitErr = outside.Wait()
				g.Task("child").Define(noop)
			})

			_ = waitWithin(t, "Finish", out.Finish)

			if !errors.Is(waitErr, graph.ErrWaitInBuilder) {
				t.Errorf("Wait inside the builder = %v, want ErrWaitInBuilder", waitErr)
			}
		})
	}
}

// A builder ordered After a Task can only run once that Task settled, so a
// Wait on it is no wait at all and is answered.
func TestSlice36_WaitInABuilderOnATaskItIsOrderedAfterIsAnswered(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 4})
	t.Cleanup(func() { _ = out.Close() })
	before := out.Task("before").Define(noop)
	var waitErr error
	out.Group("builder").After(before).Define(func(g *GroupHandle) {
		waitErr = before.Wait()
		g.Task("child").Define(noop)
	})

	_ = waitWithin(t, "Finish", out.Finish)

	if waitErr != nil {
		t.Errorf("Wait inside the builder on its own predecessor = %v, want nil", waitErr)
	}
	if misuse := out.firstMisuse(); misuse != nil {
		t.Errorf("recorded misuse = %v, want none", misuse)
	}
}
