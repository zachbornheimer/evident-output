// Command live-progress demos multi-task live progress (determinate bars +
// indeterminate spinner phases) with real wall-clock sleeps.
//
// Modes:
//
//	default (TTY)  — ANSI live region on stderr (in-place redraw)
//	--frames       — print each live snapshot with a header (scrubable log)
//	--step         — like --frames, but wait for Enter between frames
//
//	go run ./examples/live-progress/
//	go run ./examples/live-progress/ --frames
//	go run ./examples/live-progress/ --step
//	go run ./examples/live-progress/ --fast
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
	"github.com/zachbornheimer/evident-output/terminal"
)

func main() {
	fast := flag.Bool("fast", false, "use 20ms steps instead of 100ms")
	frames := flag.Bool("frames", false, "dump every live frame with a numbered header (no in-place ANSI)")
	step := flag.Bool("step", false, "frame dump + wait for Enter between frames")
	workDir := flag.String("work-dir", "", "directory for demo temp files (default: TempDir)")
	colorFlag := flag.String("color", "auto", "color final report: auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: live-progress [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Interactive multi-progress demo.\n\n")
		fmt.Fprintf(os.Stderr, "  Default: live ANSI region on stderr (in-place).\n")
		fmt.Fprintf(os.Stderr, "  --frames: print each redraw so you can scroll/scrub.\n")
		fmt.Fprintf(os.Stderr, "  --step:   --frames + press Enter between redraws.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *step {
		*frames = true
	}
	// Non-TTY stderr cannot show in-place CSI redraws; auto-switch to frame log
	// unless the user already asked for --frames/--step.
	if !*frames && !isTerminal(os.Stderr) {
		fmt.Fprintln(os.Stderr, "live-progress: stderr is not a TTY; using --frames (pass a real terminal for in-place live UI)")
		*frames = true
	}

	stepDur := 100 * time.Millisecond
	if *fast {
		stepDur = 20 * time.Millisecond
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

	var term evo.TerminalDriver
	if *frames {
		term = newFrameLog(os.Stderr, *step)
	} else {
		term = terminal.NewANSI(os.Stderr,
			terminal.WithInteractive(true),
			terminal.WithSize(80, 24),
		)
	}

	opts := []evo.Option{
		evo.Terminal(term),
		evo.DebugLevel(evo.Debug),
		evo.VisibilityDelay(0),
		evo.MaxFrameRate(60),
	}
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

	const nFiles = 24
	for i := 0; i < nFiles; i++ {
		path := filepath.Join(dir, fmt.Sprintf("pkg-%02d.tgz", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("payload-%d", i)), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	jobs := out.Tasks("dependencies")

	// Indeterminate discovery
	discover := jobs.Task("discover")
	for _, phase := range []string{"reading lockfile", "resolving graph", "planning fetch"} {
		discover.Phase(phase)
		time.Sleep(stepDur * 3)
	}
	discover.Donef("%d packages", nFiles)

	scan := jobs.Task("scan")
	download := jobs.Task("download")
	verify := jobs.Task("verify")

	const totalBytes int64 = 18_000_000
	download.Bytes(0, totalBytes)
	verify.Phase("waiting for download")

	entries, _ := os.ReadDir(dir)
	for i := range entries {
		scan.Progress(i+1, len(entries))
		done := int64(float64(totalBytes) * float64(i+1) / float64(len(entries)))
		download.Bytes(done, totalBytes)
		time.Sleep(stepDur)
	}
	scan.Done()
	download.Bytes(totalBytes, totalBytes)
	download.Donef("%.1f MB", float64(totalBytes)/(1000*1000))

	for _, phase := range []string{"checking signatures", "checksums", "quarantine scan"} {
		verify.Phase(phase)
		time.Sleep(stepDur * 4)
	}
	verify.Done()

	// Diagnostic above residual items (streamed once; dim when color is on).
	out.Debug("cache warm", evo.String("dir", dir))
	out.Item("lockfile").OK()
	out.Item("registry").OK()

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	// Human stream already owns stderr (live + durable + conclusion).
	// Do not re-dump RenderPlain — that was a second full report.
	os.Exit(out.Conclusion().ExitCode)
}

// frameLog is a LiveSurface that prints each redraw as a numbered scrubable frame
// instead of using in-place ANSI (so mise run / pipes / logs are watchable).
type frameLog struct {
	w     io.Writer
	step  bool
	n     int
	width int
	in    *bufio.Reader
}

func newFrameLog(w io.Writer, step bool) *frameLog {
	return &frameLog{w: w, step: step, width: 80, in: bufio.NewReader(os.Stdin)}
}

func (f *frameLog) ID() string          { return "frame-log" }
func (f *frameLog) Columns() int        { return f.width }
func (f *frameLog) Rows() int           { return 24 }
func (f *frameLog) IsInteractive() bool { return true }
func (f *frameLog) ClearLive()          {}
func (f *frameLog) WriteDurable(line string) {
	fmt.Fprintf(f.w, "  · durable: %s\n", line)
}
func (f *frameLog) WriteFinal(text string) {
	fmt.Fprintf(f.w, "\n── final ──\n%s\n", text)
}
func (f *frameLog) WriteLive(text string) {
	f.n++
	fmt.Fprintf(f.w, "\n── frame %d ──\n%s\n", f.n, text)
	if f.step {
		fmt.Fprint(f.w, "\n[Enter] next frame, or q+Enter to skip stepping… ")
		line, _ := f.in.ReadString('\n')
		if len(line) > 0 && (line[0] == 'q' || line[0] == 'Q') {
			f.step = false
		}
	}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
