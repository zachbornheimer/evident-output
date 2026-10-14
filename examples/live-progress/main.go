// Command live-progress is the ordinary multi-progress demo (user API only).
//
//	go run ./examples/live-progress/
//	go run ./examples/live-progress/ --fast
//
// Advanced frame logging / custom TerminalDriver: see examples/terminal-driver.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	fast := flag.Bool("fast", false, "use short sleeps")
	flag.Parse()

	step := 80 * time.Millisecond
	if *fast {
		step = 15 * time.Millisecond
	}

	os.Exit(evo.Main(evo.Config{Name: "install dependencies"}, func(ctx context.Context, r *evo.Run) error {
		return runLive(r, step)
	}))
}

// runLive uses evo.Sequence: dependencies is a sequence of steps that must stop
// on failure, and evo-rec.md's dialect for that shape is sequential
// presentation — one Running child at a time, later siblings named and
// idle until their turn (the "python" example). Each step below predeclares
// its handle, then fully resolves before the next step's Phase/Progress/Bytes
// call promotes it to Running.
func runLive(r *evo.Run, step time.Duration) error {
	const packageCount = 24
	const totalBytes int64 = 18_000_000

	jobs := r.Sequence("dependencies")
	discover := jobs.Task("discover")
	scan := jobs.Task("scan")
	download := jobs.Task("download")
	verify := jobs.Task("verify")

	discover.Define(func(ctx context.Context) error {
		for _, phase := range []string{"reading lockfile", "resolving graph", "planning fetch"} {
			discover.Doing(phase)
			time.Sleep(step * 2)
		}
		return nil
	})

	scan.Define(func(ctx context.Context) error {
		for completed := 1; completed <= packageCount; completed++ {
			scan.Progress(completed, packageCount)
			time.Sleep(step)
		}
		return nil
	})

	download.Define(func(ctx context.Context) error {
		for completed := 1; completed <= packageCount; completed++ {
			done := totalBytes * int64(completed) / int64(packageCount)
			download.Progress(int(done), int(totalBytes))
			time.Sleep(step)
		}
		return nil
	})

	verify.Define(func(ctx context.Context) error {
		for _, phase := range []string{"checking signatures", "checksums", "quarantine scan"} {
			verify.Doing(phase)
			time.Sleep(step * 2)
		}
		return nil
	})

	r.Task("lockfile").Define(func(context.Context) error { return nil })
	r.Task("registry").Define(func(context.Context) error { return nil })
	return nil
}
