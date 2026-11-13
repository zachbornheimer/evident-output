// Command debug-pane demos a sequential audit with an optional blocker.
//
//	go run ./examples/debug-pane/
//	go run ./examples/debug-pane/ --fail
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
	fail := flag.Bool("fail", false, "find a blocker")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	evo.Init(evo.Config{Title: "branch audit"})
	os.Exit(evo.Main(func(ctx context.Context) error {
		jobs := evo.Sequence("audit")
		scan := jobs.Task("scan")
		compare := jobs.Task("compare")

		scan.Define(func(context.Context) error {
			scan.Doing("enumerating")
			time.Sleep(step)
			scan.Summary("7 branches")
			return nil
		})
		compare.Define(func(context.Context) error {
			compare.Doing("diffing")
			time.Sleep(step)
			if *fail {
				compare.Summary("1 blocker found")
			}
			return nil
		})
		if err := compare.Wait(); err != nil {
			return err
		}

		branches := evo.Task("branches")
		if *fail {
			branches.Block("feat/sdk-full-consolidation is local-only")
		} else {
			branches.Define(func(context.Context) error { return nil })
		}
		return nil
	}))
}
