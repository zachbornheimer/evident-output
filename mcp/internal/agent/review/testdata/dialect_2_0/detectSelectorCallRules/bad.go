package fixture

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

// run prints straight to stdout while Evo owns the live region, then exits
// with a hand-picked code instead of the run's conclusion.
func run(ctx context.Context, r *evo.Run) error {
	scan := r.Task("scan sources")
	scan.Define(func(ctx context.Context) error {
		fmt.Println("still scanning")
		return nil
	})
	if ctx.Err() != nil {
		os.Exit(1)
	}
	return nil
}
