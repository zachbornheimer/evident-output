// Command many-tasks demos one Group of many trivial Tasks that all stay
// Running for the same hold, so the live region has a large census to animate.
//
//	go run ./examples/many-tasks/ --tasks 10000
//
// With --late, a free root Task also runs, starting once the Group's last
// Task settled and holding for that long, so a root collection and a root row
// that settles after it can be compared.
//
// tools/verify-quiet-pty-stream.py --tasks N runs it under a real pty to prove
// the live frame keeps changing at that scale.
package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// groupPoll is how often the late Task checks whether the Group settled.
const groupPoll = time.Millisecond

func main() {
	count := flag.Int("tasks", 1000, "how many Tasks to declare in the Group")
	hold := flag.Duration("hold", 5*time.Millisecond, "how long each Task stays Running")
	late := flag.Duration("late", 0, "when above 0, also run a root Task that holds this long after the Group settled")
	flag.Parse()

	os.Exit(evo.Main(evo.Config{}, func(ctx context.Context, r *evo.Run) error {
		group := r.Group("units")
		var running atomic.Int64
		running.Store(int64(*count))
		for n := 1; n <= *count; n++ {
			unit := group.Task("unit " + strconv.Itoa(n))
			unit.Define(func(ctx context.Context) error {
				defer running.Add(-1)
				return holdRunning(ctx, unit, *hold)
			})
		}
		if *late > 0 {
			lateTask := r.Task("late task")
			lateTask.Define(func(ctx context.Context) error {
				if err := waitUntilZero(ctx, &running); err != nil {
					return err
				}
				return holdRunning(ctx, lateTask, *late)
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

// waitUntilZero returns once n reaches zero, or the context ends.
func waitUntilZero(ctx context.Context, n *atomic.Int64) error {
	for n.Load() > 0 {
		select {
		case <-time.After(groupPoll):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
