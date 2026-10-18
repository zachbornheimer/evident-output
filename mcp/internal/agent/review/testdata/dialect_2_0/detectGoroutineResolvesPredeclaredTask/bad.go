package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	task := r.Task("sync mirrors")
	go func() {
		task.Doing("copying")
		task.Progress(1, 2)
	}()
	return nil
}
