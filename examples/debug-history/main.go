// Command debug-history demos a short sequential probe.
//
//	go run ./examples/debug-history/ --fast
package main

import (
	"context"
	"flag"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	flag.Parse()
	step := 120 * time.Millisecond
	if *fast {
		step = 25 * time.Millisecond
	}

	os.Exit(evo.Main(evo.Config{Name: "repo-probe"}, func(ctx context.Context, r *evo.Run) error {
		r.Task("working tree").Define(func(context.Context) error {
			time.Sleep(step)
			return nil
		})
		r.Task("branches").Define(func(context.Context) error {
			time.Sleep(step)
			return nil
		})
		return nil
	}))
}
