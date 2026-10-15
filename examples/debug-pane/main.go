// Command debug-pane demos a sequential audit with an optional blocker. With
// --fail the compare step returns evo.Refuse: the run exits 1 and the Task
// declared After the audit never starts.
//
//	go run ./examples/debug-pane/
//	go run ./examples/debug-pane/ --fail
package main

import (
	"context"
	"flag"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	fail := flag.Bool("fail", false, "find a blocker")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	os.Exit(evo.Main(evo.Config{Name: "branch audit"}, func(ctx context.Context, r *evo.Run) error {
		jobs := r.Sequence("audit")
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
				return evo.Refuse("feat/sdk-full-consolidation is local-only",
					evo.Detail("push, merge, or delete local-only work before retiring this repository"))
			}
			return nil
		})

		r.Task("branches").After(jobs).Define(func(context.Context) error { return nil })
		return nil
	}))
}
