// The Build callback reads the filesystem, starts a goroutine and uses the
// run context: three jobs a builder must leave to Tasks.
package api069

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) {
	group := r.Group("centralize packages")
	group.Build(func(g *evo.Group) {
		entries, _ := os.ReadDir("/opt/packages")
		go poll(ctx)
		for _, e := range entries {
			g.Task(e.Name()).Define(func(ctx context.Context) error { return nil })
		}
	})
}

func poll(ctx context.Context) {}
