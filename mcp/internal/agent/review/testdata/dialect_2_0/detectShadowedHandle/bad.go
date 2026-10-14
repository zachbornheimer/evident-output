package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	t := r.Task("scan")
	t.Doing("walking")
	t = r.Task("build")
	t.Define(build)
	return nil
}

func build(ctx context.Context) error { return nil }
