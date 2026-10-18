package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("check file integrity").Define(verifyIntegrity)
	r.Group("fix").Task("format Go source").Define(verifyIntegrity)
	return nil
}

func verifyIntegrity(ctx context.Context) error { return nil }
