package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("publish").Define(func(ctx context.Context) error {
		return evo.Exclude("protected branch")
	})
	return nil
}
