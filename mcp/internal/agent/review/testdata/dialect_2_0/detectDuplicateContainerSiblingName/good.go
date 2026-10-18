package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Group("build").Task("compile").Define(compile)
	r.Sequence("package").Task("link").Define(link)
	r.Task("build").Define(finish)
	return nil
}

func compile(ctx context.Context) error { return nil }
func link(ctx context.Context) error    { return nil }
func finish(ctx context.Context) error  { return nil }
