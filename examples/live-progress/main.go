// Command live-progress demos multi-task live progress (determinate bars +
// indeterminate spinner phases) with real wall-clock sleeps.
//
// Uses the production ANSI driver on stderr so you see the live region redraw.
// Do not pass Plain() — that disables the interactive live surface.
//
//	go run ./examples/live-progress/
//	go run ./examples/live-progress/ --fast          # shorter sleeps
//	go run ./examples/live-progress/ --work-dir /tmp/foo
//
// What you'll see:
//   - several concurrent-looking tasks under a Tasks collection
//   - determinate unit progress bars (scan files)
//   - determinate byte progress bars (download)
//   - indeterminate phase spinner (verify signatures)
//   - debug lines inserted above the live region
//   - a final static report after Finish
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
	"github.com/zachbornheimer/evident-output/terminal"
)

func main() {
	fast := flag.Bool("fast", false, "use 20ms steps instead of 100ms (CI-friendly)")
	workDir := flag.String("work-dir", "", "directory to write demo temp files (default: os.TempDir()/evo-live-progress-*)")
	colorFlag := flag.String("color", "auto", "color final report: auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: live-progress [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Interactive multi-progress demo (live region on stderr).\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	dir := *workDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "evo-live-progress-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer os.RemoveAll(dir)
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Live UI owns stderr (ANSI live region). Final plain report also goes there.
	// SystemClock + real Sleep so frames advance over wall time.
	drv := terminal.NewANSI(os.Stderr,
		terminal.WithInteractive(true),
		terminal.WithSize(80, 24),
	)
	opts := []evo.Option{
		evo.Terminal(drv),
		evo.DebugLevel(evo.Debug),
		evo.VisibilityDelay(0), // show progress immediately
		evo.MaxFrameRate(30),
	}
	// Final report color policy (live glyphs stay monochrome in the driver today).
	switch demo.ParseColorFlag(*colorFlag) {
	case demo.ColorNever:
		opts = append(opts, evo.NoColor())
	case demo.ColorAuto:
		if os.Getenv("NO_COLOR") != "" {
			opts = append(opts, evo.NoColor())
		}
	}
	out := evo.For("install-deps", opts...)
	defer out.Close()

	// Seed a few files to "scan".
	const nFiles = 24
	for i := 0; i < nFiles; i++ {
		path := filepath.Join(dir, fmt.Sprintf("pkg-%02d.tgz", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("payload-%d", i)), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	jobs := out.Tasks("dependencies")

	// 1) Indeterminate while discovering packages.
	discover := jobs.Task("discover")
	for _, phase := range []string{"reading lockfile", "resolving graph", "planning fetch"} {
		discover.Phase(phase)
		time.Sleep(step * 3)
	}
	discover.Donef("%d packages", nFiles)

	// 2) Determinate unit progress — multi-bar feel with siblings running later.
	scan := jobs.Task("scan")
	download := jobs.Task("download")
	verify := jobs.Task("verify") // indeterminate until later

	// Start download early so bars appear together.
	const totalBytes int64 = 18_000_000
	download.Bytes(0, totalBytes)
	verify.Phase("waiting for download")

	// Interleave scan (unit bar) and download (byte bar).
	entries, _ := os.ReadDir(dir)
	for i, e := range entries {
		scan.Progress(int64(i+1), int64(len(entries)))
		// Download advances in chunks each step.
		done := int64(float64(totalBytes) * float64(i+1) / float64(len(entries)))
		download.Bytes(done, totalBytes)
		_ = e
		time.Sleep(step)
	}
	scan.Done()
	download.Bytes(totalBytes, totalBytes)
	download.Donef("%s", formatMB(totalBytes))

	// 3) Indeterminate verify phases after determinate work.
	for _, phase := range []string{"checking signatures", "checksums", "quarantine scan"} {
		verify.Phase(phase)
		time.Sleep(step * 4)
	}
	verify.Done()

	// Durable log above live region (clears → writes → redraws while live was active;
	// after children complete this still journals).
	out.Debug("cache warm", evo.String("dir", dir))

	out.Item("lockfile").OK()
	out.Item("registry").OK()

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	// WriteFinal already painted the interactive final; also print a colored
	// plain summary to stdout for copy/paste in logs.
	snap := out.Snapshot()
	noColor := demo.ParseColorFlag(*colorFlag) == demo.ColorNever ||
		(demo.ParseColorFlag(*colorFlag) == demo.ColorAuto && os.Getenv("NO_COLOR") != "")
	plain, _ := evo.RenderPlain(snap, evo.PlainOptions{Width: 80, NoColor: noColor})
	fmt.Fprintln(os.Stdout)
	_, _ = os.Stdout.Write(plain)

	os.Exit(out.Conclusion().ExitCode)
}

func formatMB(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/(1000*1000))
}
