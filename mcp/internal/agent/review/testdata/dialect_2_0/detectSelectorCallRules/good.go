package fixture

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

// run writes human text through the run and leaves the exit code to Main.
func run(ctx context.Context, r *evo.Run) error {
	scan := r.Task("scan sources")
	scan.Define(func(ctx context.Context) error {
		fmt.Fprintln(r.Text(), "still scanning")
		return nil
	})
	return ctx.Err()
}
