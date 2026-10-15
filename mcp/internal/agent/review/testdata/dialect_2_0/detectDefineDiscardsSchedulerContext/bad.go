package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(context.Context) error {
		return syncMirrors(ctx)
	}, evo.CleanStop(os.Interrupt))
	return nil
}

func syncMirrors(ctx context.Context) error { return nil }
