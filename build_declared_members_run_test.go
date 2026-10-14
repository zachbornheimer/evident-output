package evo

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
	"testing"
)

// Members a Build callback declared before it exited through runtime.Goexit or
// panicked still run, even where none had started when the builder died
// (MaxConcurrency 1): the container fails, but the work it already declared is
// not withdrawn (CONTRACT section 2).
func TestBuildMembersDeclaredBeforeTheBuilderDiedStillRun(t *testing.T) {
	dies := map[string]func(){
		"Goexit": runtime.Goexit,
		"panic":  func() { panic("builder boom") },
	}
	for how, die := range dies {
		for _, concurrency := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/max%d", how, concurrency), func(t *testing.T) {
				var group, sequence atomic.Bool
				Execute(context.Background(), Config{Output: Plain, MaxConcurrency: concurrency, Facades: Facades{Stderr: io.Discard}},
					func(ctx context.Context, r *Run) error {
						r.Group("g").Build(func(g *Group) {
							g.Task("member").Define(func(context.Context) error { group.Store(true); return nil })
							die()
						})
						r.Sequence("s").Build(func(s *Sequence) {
							s.Task("one").Define(func(context.Context) error { sequence.Store(true); return nil })
							die()
						})
						return nil
					})
				if !group.Load() || !sequence.Load() {
					t.Errorf("max %d: a member declared before the builder died ran: Group %v, Sequence %v; want both", concurrency, group.Load(), sequence.Load())
				}
			})
		}
	}
}
