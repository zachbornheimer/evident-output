package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("fix").Define(func(ctx context.Context) error {
		if err := fixImports(ctx); err != nil {
			return err
		}
		if err := fixFormatting(ctx); err != nil {
			return err
		}
		return nil
	})
	return nil
}

func fixImports(ctx context.Context) error    { return nil }
func fixFormatting(ctx context.Context) error { return nil }
