package fixture

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// main builds its own interrupt context although Main already owns it.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(evo.Main(evo.Config{}, func(_ context.Context, r *evo.Run) error { return run(ctx, r) }))
}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(ctx context.Context) error { return ctx.Err() })
	return nil
}
