// Command many-tasks demos one Group of many trivial Tasks that all stay
// Running for the same hold, so the live region has a large census to animate.
//
//	go run ./examples/many-tasks/ --tasks 10000
//
// tools/verify-quiet-pty-stream.py --tasks N runs it under a real pty to prove
// the live frame keeps changing at that scale.
package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	count := flag.Int("tasks", 1000, "how many Tasks to declare in the Group")
	hold := flag.Duration("hold", 5*time.Millisecond, "how long each Task stays Running")
	flag.Parse()

	os.Exit(evo.Main(func(ctx context.Context, r *evo.Run) error {
		group := r.Group("units")
		for n := 1; n <= *count; n++ {
			unit := group.Task("unit " + strconv.Itoa(n))
			unit.Define(func(ctx context.Context) error {
				return holdRunning(ctx, unit, *hold)
			})
		}
		return nil
	}))
}

// holdRunning keeps the Task Running with no output until hold elapses.
func holdRunning(ctx context.Context, unit *evo.Task, hold time.Duration) error {
	unit.Doing("holding")
	select {
	case <-time.After(hold):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
