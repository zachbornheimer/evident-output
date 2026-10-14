package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// main drops the exit code, so a failed run still exits 0.
func main() {
	evo.Main(evo.Config{}, run)
}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(ctx context.Context) error { return evo.Fail("mirror is unreachable") })
	return nil
}
