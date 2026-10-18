package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	prune := r.Group("prune")
	branches := prune.Task("branches").Define(pruneBranches)
	prune.Task("remote-tracking").After(branches).Define(pruneRemote)
	return nil
}

func pruneBranches(ctx context.Context) error { return nil }
func pruneRemote(ctx context.Context) error   { return nil }
