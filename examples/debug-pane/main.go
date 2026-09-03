// Command debug-pane demos rolling slog-text viewport.
//
//	go run ./examples/debug-pane/
//	go run ./examples/debug-pane/ --fail
package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	fail := flag.Bool("fail", false, "find a blocker (task Done + item Block)")
	preserve := flag.Bool("preserve", false, "always keep diagnostic tail")
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	newest := true
	cfg := evo.DefaultConfig()
	cfg.Title = "branch audit"
	cfg.Debug = evo.DebugConfig{
		Level:          evo.Debug,
		View:           evo.DebugPresentationPane,
		PaneHeight:     4,
		NewestFirst:    &newest,
		PreserveAlways: *preserve,
	}
	out := evo.Init(cfg)
	log := slog.New(out.SlogHandler())

	os.Exit(evo.Main(func() error {
		jobs := evo.Group("audit")
		scan := jobs.Task("scan")
		compare := jobs.Task("compare")

		scan.Phase("enumerating")
		log.Debug("enumerated local branches", "count", 7)
		time.Sleep(step)
		log.Debug("fetched remote metadata", "remote", "origin")
		time.Sleep(step)
		scan.Donef("%d branches", 7)

		compare.Phase("diffing")
		blockers := 0
		if *fail {
			blockers = 1
		}
		log.Debug("branch comparison completed", "blockers", blockers)
		time.Sleep(step)
		log.Debug("policy check", "rule", "no-local-only")
		time.Sleep(step)
		if *fail {
			// Comparison succeeded and found a domain blocker — not an operation failure.
			compare.Done("1 blocker found")
			evo.Item("branches").Block("feat/sdk-full-consolidation is local-only")
		} else {
			compare.Done()
			evo.Item("branches").OK()
		}
		return nil
	}))
}
