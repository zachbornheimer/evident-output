// Command debug-history demos a short sequential probe.
//
//	go run ./examples/debug-history/ --fast
package main

import (
	"flag"
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
	evo.Main(func() error {
		time.Sleep(step)
		evo.Task("working tree").Done()
		time.Sleep(step)
		evo.Task("branches").Done()
		return nil
	})
}
