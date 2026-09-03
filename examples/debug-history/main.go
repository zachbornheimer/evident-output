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

	out := evo.Init(evo.Config{
		Title: "repo-probe",
		Debug: evo.DebugConfig{Level: evo.Debug},
	})
	log := slog.New(out.SlogHandler())

	os.Exit(evo.Main(func() error {
		time.Sleep(step)
		log.Debug("opened repository", "path", "/work/bpp-csharp")
		evo.Item("working tree").OK()

		time.Sleep(step)
		log.Debug("enumerated local branches", "count", 7)
		time.Sleep(step)
		log.Debug("branch comparison completed", "blockers", 0, "duration", 11*time.Millisecond)
		evo.Item("branches").OK()
		return nil
	}))
}
