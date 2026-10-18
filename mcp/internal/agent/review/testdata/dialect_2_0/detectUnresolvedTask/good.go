package fixture

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	t := r.Task("fetch origin")
	t.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "git", "fetch")
		cmd.Stdout = t.Writer()
		return cmd.Run()
	})
	return nil
}
