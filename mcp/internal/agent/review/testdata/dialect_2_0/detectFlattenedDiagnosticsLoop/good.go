package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type finding struct{ File, Message, Code string }

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	task := r.Task("lint staged files")
	task.Define(func(ctx context.Context) error {
		for _, f := range scan() {
			task.Problem(f.Message, evo.On(f.File), evo.Code(f.Code))
		}
		return nil
	})
	return nil
}

func scan() []finding { return nil }
