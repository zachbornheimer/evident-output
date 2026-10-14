package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// main hands the run's exit code to the process.
func main() {
	os.Exit(evo.Main(evo.Config{}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(ctx context.Context) error { return evo.Fail("mirror is unreachable") })
	return nil
}
