package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type finding struct{ File, Message string }

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	issues := r.Group("integrity issues")
	for _, f := range scan() {
		issues.Task(f.File).Define(func(ctx context.Context) error {
			return evo.Fail(f.Message)
		})
	}
	return nil
}

func scan() []finding { return nil }
