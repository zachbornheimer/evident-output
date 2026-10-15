package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("file integrity").Define(verifyIntegrity)
	r.Task("fix").Define(verifyIntegrity)
	return nil
}

func verifyIntegrity(ctx context.Context) error { return nil }
