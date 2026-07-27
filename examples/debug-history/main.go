// Command debug-history demos v0.4 DebugHistory (default): durable scrollback
// above the live region using the compact bracketed grammar.
//
//	go run ./examples/debug-history/
//	go run ./examples/debug-history/ --fast
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	colorFlag := flag.String("color", "auto", "auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: debug-history [flags]\n\n")
		fmt.Fprintf(os.Stderr, "History-mode debug: each Debug line becomes durable scrollback\n")
		fmt.Fprintf(os.Stderr, "above the live region (HH:MM:SS.mmm [DEBUG] msg key=val).\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 120 * time.Millisecond
	if *fast {
		step = 25 * time.Millisecond
	}

	out := evo.For("repo-probe",
		demo.Options(os.Stderr, demo.ParseColorFlag(*colorFlag),
			evo.DebugLevel(evo.Debug),
			evo.DebugHistory(),
		)...,
	)
	os.Exit(evo.Main(out, func(o *evo.Output) error {
		time.Sleep(step)
		o.Debug("opened repository", evo.String("path", "/work/bpp-csharp"))
		o.Item("working tree").OK()

		time.Sleep(step)
		o.Debug("enumerated local branches", evo.Int("count", 7))
		time.Sleep(step)
		o.Debug("branch comparison completed", evo.Int("blockers", 0), evo.String("duration", "11ms"))
		o.Item("branches").OK()
		return nil
	}))
}
