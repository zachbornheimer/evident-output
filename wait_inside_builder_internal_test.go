package evo

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
	waitedOn := map[string]func(o *Run, g *Group) func() error{
		"a Task declared outside the builder": func(o *Run, _ *Group) func() error {
			outside := o.Task("outside").Define(func(context.Context) error { time.Sleep(5 * time.Millisecond); return nil })
			return outside.wait
		},
		"a child the builder declared": func(_ *Run, g *Group) func() error {
			child := g.Task("child").Define(func(context.Context) error { return nil })
			return child.wait
		},
		"its own group": func(_ *Run, g *Group) func() error {
			g.Task("child").Define(func(context.Context) error { return nil })
			return g.wait
		},
	}
	for name, declare := range waitedOn {
		for _, ceiling := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/ceiling=%d", name, ceiling), func(t *testing.T) {
				out := quietOutput(Config{MaxConcurrency: ceiling})
				t.Cleanup(func() { _ = out.close() })
				var waitErr error
				out.Group("builder").define(func(g *Group) {
					waitErr = declare(out, g)()
				})

				_ = waitWithin(t, "Finish", out.finish)

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
