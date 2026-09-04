// Command live-progress is the ordinary multi-progress demo (user API only).
//
//	go run ./examples/live-progress/
//	go run ./examples/live-progress/ --fast
//
// Advanced frame logging / custom TerminalDriver: see examples/terminal-driver.
package main

import (
	"flag"
	"log/slog"
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

	out := evo.Init(evo.Config{
		Title: "install dependencies",
		Debug: evo.DebugConfig{Level: evo.LevelDebug},
	})
	log := slog.New(out.SlogHandler())

	evo.Main(func() error {
		return runLive(out, log, step)
	})
}

// runLive uses evo.Sequence: dependencies is a sequence of steps that must stop
// on failure, and evo-rec.md's dialect for that shape is sequential
// presentation — one Running child at a time, later siblings named and
// idle until their turn (the "python" example). Each step below predeclares
// its handle, then fully resolves before the next step's Phase/Progress/Bytes
// call promotes it to Running.
func runLive(out *evo.Output, log *slog.Logger, step time.Duration) error {
	const packageCount = 24
	const totalBytes int64 = 18_000_000

	jobs := out.Sequence("dependencies")
	discover := jobs.Task("discover")
	scan := jobs.Task("scan")
	download := jobs.Task("download")
	verify := jobs.Task("verify")

	for _, phase := range []string{"reading lockfile", "resolving graph", "planning fetch"} {
		discover.Phase(phase)
		time.Sleep(step * 2)
	}
	discover.Done("%d packages", packageCount)

	for completed := 1; completed <= packageCount; completed++ {
		scan.Progress(completed, packageCount)
		time.Sleep(step)
	}
	scan.Done()

	for completed := 1; completed <= packageCount; completed++ {
		done := totalBytes * int64(completed) / int64(packageCount)
		download.Bytes(done, totalBytes)
		if completed == packageCount/2 {
			log.Debug("download midpoint", "bytes", done, "of", totalBytes)
		}
		time.Sleep(step)
	}
	download.Done("%.1f MB", float64(totalBytes)/(1000*1000))

	for _, phase := range []string{"checking signatures", "checksums", "quarantine scan"} {
		verify.Phase(phase)
		time.Sleep(step * 2)
	}
	verify.Done()

	log.Debug("dependency graph resolved", "packages", packageCount)
	evo.Task("lockfile").Done()
	evo.Task("registry").Done()
	return nil
}
