package fixture

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	go func() {
		for range reload {
			reloadConfig()
		}
	}()
	os.Exit(evo.Main(evo.Config{}, run))
}

func reloadConfig() {}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(ctx context.Context) error { return ctx.Err() })
	return nil
}
