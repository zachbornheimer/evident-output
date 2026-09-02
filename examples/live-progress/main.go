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

	out := evo.New(evo.Config{
		Title: "install dependencies",
		Debug: evo.DebugConfig{Level: evo.Debug},
	})
	log := slog.New(out.SlogHandler())

	os.Exit(evo.MainWith(out, func(o *evo.Output) error {
		return runLive(o, log, step)
	}))
}

func runLive(out *evo.Output, log *slog.Logger, step time.Duration) error {
	const packageCount = 24
	const totalBytes int64 = 18_000_000

	jobs := out.Tasks("dependencies")
	discover := jobs.Task("discover")
	scan := jobs.Task("scan")
	download := jobs.Task("download")
	verify := jobs.Task("verify")

	for _, phase := range []string{"reading lockfile", "resolving graph", "planning fetch"} {
		discover.Phase(phase)
		time.Sleep(step * 2)
	}
	discover.Donef("%d packages", packageCount)

	download.Bytes(0, totalBytes)
	verify.Phase("waiting for download")
	for completed := 1; completed <= packageCount; completed++ {
		scan.Progress(completed, packageCount)
		done := totalBytes * int64(completed) / int64(packageCount)
		download.Bytes(done, totalBytes)
		if completed == packageCount/2 {
			log.Debug("download midpoint", "bytes", done, "of", totalBytes)
		}
		time.Sleep(step)
	}
	scan.Done()
	download.Bytes(totalBytes, totalBytes)
	download.Donef("%.1f MB", float64(totalBytes)/(1000*1000))

	for _, phase := range []string{"checking signatures", "checksums", "quarantine scan"} {
		verify.Phase(phase)
		time.Sleep(step * 2)
	}
	verify.Done()

	log.Debug("dependency graph resolved", "packages", packageCount)
	out.Item("lockfile").OK()
	out.Item("registry").OK()
	return nil
}
