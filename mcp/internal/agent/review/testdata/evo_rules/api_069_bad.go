// Fixture: API-069 must fire four ways. The builder reads the filesystem,
// starts a goroutine, waits, and returns an error.
package api069

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	group := out.Group("centralize packages")
	group.Define(func(g *evo.GroupHandle) {
		entries, _ := os.ReadDir("/opt/packages")
		go poll()
		g.Wait()
		for _, e := range entries {
			g.Task(e.Name()).Define(func(ctx context.Context) error { return nil })
		}
	})
	group.Define(func(g *evo.GroupHandle) error { return nil })
}
