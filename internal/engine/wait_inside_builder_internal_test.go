package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// A container builder declares the topology the run is built from; the Tasks
// and containers it waits on cannot settle before it does, so a Wait inside
// it parks forever and Finish hangs behind it. The Wait is refused instead,
// and the refusal is recorded as misuse.
func TestWaitInsideABuilderIsRefusedAndFinishReturns(t *testing.T) {
	waitedOn := map[string]func(o *Output, g *GroupHandle) func() error{
		"a Task declared outside the builder": func(o *Output, _ *GroupHandle) func() error {
			outside := o.Task("outside").Define(func(context.Context) error { time.Sleep(5 * time.Millisecond); return nil })
			return outside.Wait
		},
		"a child the builder declared": func(_ *Output, g *GroupHandle) func() error {
			child := g.Task("child").Define(func(context.Context) error { return nil })
			return child.Wait
		},
		"its own group": func(_ *Output, g *GroupHandle) func() error {
			g.Task("child").Define(func(context.Context) error { return nil })
			return g.Wait
		},
	}
	for name, declare := range waitedOn {
		for _, ceiling := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/ceiling=%d", name, ceiling), func(t *testing.T) {
				out := quietOutput(Config{MaxConcurrency: ceiling})
				t.Cleanup(func() { _ = out.Close() })
				var waitErr error
				out.Group("builder").Define(func(g *GroupHandle) {
					waitErr = declare(out, g)()
				})

				_ = waitWithin(t, "Finish", out.Finish)

				if !errors.Is(waitErr, graph.ErrWaitInBuilder) {
					t.Errorf("Wait inside the builder = %v, want ErrWaitInBuilder", waitErr)
				}
				if misuse := out.firstMisuse(); !errors.Is(misuse, graph.ErrWaitInBuilder) {
					t.Errorf("recorded misuse = %v, want ErrWaitInBuilder", misuse)
				}
			})
		}
	}
}
