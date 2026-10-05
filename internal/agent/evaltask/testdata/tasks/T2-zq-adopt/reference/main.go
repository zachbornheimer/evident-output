package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"

	"evalsandbox/fixture"
)

func main() {
	evo.Init(evo.Config{Title: "adopt"})
	os.Exit(evo.Main(run))
}

func run(context.Context) error {
	declareAdopt(evo.Default())
	return nil
}

func declareAdopt(out *evo.Output) {
	adopt := out.Sequence("adopt")
	repositories := evo.Compute(adopt.Task("find repositories"), fixture.Repositories)
	adopt.Group("adopt repositories").Define(func(each *evo.GroupHandle) {
		for _, repository := range repositories.Get() {
			each.Task("adopt " + repository).Define(func(ctx context.Context) error {
				return fixture.Adopt(ctx, repository)
			})
		}
	})
}
