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

	evo.Init(evo.Config{Title: "repo-probe"})
	os.Exit(evo.Main(func(ctx context.Context) error {
		evo.Task("working tree").Define(func(context.Context) error {
			time.Sleep(step)
			return nil
		})
		evo.Task("branches").Define(func(context.Context) error {
			time.Sleep(step)
			return nil
		})
		return nil
	}))
}
