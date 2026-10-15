package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	build := r.Group("build")
	build.Task("compile").Define(compile)
	build.Task("vet").Define(compile)
	again := r.Group("build")
	again.Task("link").Define(link)
	again.Task("sign").Define(link)
	return nil
}

func compile(ctx context.Context) error { return nil }
func link(ctx context.Context) error    { return nil }
