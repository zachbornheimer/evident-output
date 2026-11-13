// Command debug-pane demos a sequential audit with an optional blocker.
//
//	go run ./examples/debug-pane/
//	go run ./examples/debug-pane/ --fail
package main

import (
	"flag"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	fail := flag.Bool("fail", false, "find a blocker")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	evo.Init(evo.Config{Title: "branch audit"})
	evo.Main(func() error {
		jobs := evo.Sequence("audit")
		scan := jobs.Task("scan")
		compare := jobs.Task("compare")

		scan.Doing("enumerating")
		time.Sleep(step)
		scan.Done("7 branches")

		compare.Doing("diffing")
		time.Sleep(step)
		if *fail {
			compare.Done("1 blocker found")
			evo.Task("branches").Block("feat/sdk-full-consolidation is local-only")
		} else {
			compare.Done()
			evo.Task("branches").Done()
		}
		return nil
	})
}
