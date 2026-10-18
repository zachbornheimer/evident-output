package fixture

import (
	"context"
	"os"
	"os/signal"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	go func() {
		<-interrupted
		os.Exit(1)
	}()
	os.Exit(evo.Main(evo.Config{}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("sync mirrors").Define(func(ctx context.Context) error { return nil })
	return nil
}
