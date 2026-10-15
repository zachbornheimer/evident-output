package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Group("build").Task("compile").Define(compile)
	r.Sequence("build").Task("link").Define(link)
	return nil
}

func compile(ctx context.Context) error { return nil }
func link(ctx context.Context) error    { return nil }
