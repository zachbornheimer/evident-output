package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// main lets Main cancel the run's context on SIGINT and SIGTERM.
func main() {
	os.Exit(evo.Main(evo.Config{}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(ctx context.Context) error { return ctx.Err() })
	return nil
}
