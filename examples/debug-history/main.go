// Command debug-history demos durable debug scrollback via slog.
//
//	go run ./examples/debug-history/ --fast
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
	flag.Parse()
	step := 120 * time.Millisecond
	if *fast {
		step = 25 * time.Millisecond
	}

	out := evo.New(evo.Config{
		Title: "repo-probe",
		Debug: evo.DebugConfig{Level: evo.Debug},
	})
	log := slog.New(out.SlogHandler(slog.LevelDebug))

	os.Exit(evo.Main(out, func(o *evo.Output) error {
		time.Sleep(step)
		log.Debug("opened repository", "path", "/work/bpp-csharp")
		o.Item("working tree").OK()

		time.Sleep(step)
		log.Debug("enumerated local branches", "count", 7)
		time.Sleep(step)
		log.Debug("branch comparison completed", "blockers", 0, "duration", 11*time.Millisecond)
		o.Item("branches").OK()
		return nil
	}))
}
