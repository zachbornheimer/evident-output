package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	deploy := r.Task("deploy")
	r.Task("announce").After(deploy).Define(func(ctx context.Context) error { return nil })
	return nil
}
