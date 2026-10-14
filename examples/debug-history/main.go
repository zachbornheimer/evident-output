// Command debug-history demos the debug journal: Config.Debug keeps slog
// records written through Run.SlogHandler, and they print durably beside the
// rows of the Tasks that wrote them.
//
//	go run ./examples/debug-history/ --fast
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	flag.Parse()
	step := 120 * time.Millisecond
	if *fast {
		step = 25 * time.Millisecond
	}

	cfg := evo.Config{Name: "repo-probe", Debug: slog.LevelDebug}
	os.Exit(evo.Main(cfg, func(ctx context.Context, r *evo.Run) error {
		log := slog.New(r.SlogHandler())
		r.Task("working tree").Define(func(context.Context) error {
			time.Sleep(step)
			log.Debug("working tree scanned", "files", 214)
			return nil
		})
		r.Task("branches").Define(func(context.Context) error {
			time.Sleep(step)
			log.Debug("branches listed", "local", 7, "remote", 12)
			return nil
		})
		return nil
	}))
}
