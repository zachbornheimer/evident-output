// Command terminal-driver is an advanced teaching surface: custom TerminalDriver,
// frame-by-frame logging, and explicit renderer knobs.
//
// Ordinary multi-progress API: see examples/live-progress.
//
//	go run ./examples/terminal-driver/
//	go run ./examples/terminal-driver/ --frames
//	go run ./examples/terminal-driver/ --step
//	go run ./examples/terminal-driver/ --fast
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

func main() {
	fast := flag.Bool("fast", false, "use 20ms steps instead of 100ms")
	frames := flag.Bool("frames", false, "dump every live frame with a numbered header")
	step := flag.Bool("step", false, "frame dump + wait for Enter between frames")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: terminal-driver [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Advanced: custom TerminalDriver / frame log.\n")
		fmt.Fprintf(os.Stderr, "For normal multi-progress, use examples/live-progress.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *step {
		*frames = true
	}
	if !*frames && !evo.IsCharDevice(os.Stderr) {
		fmt.Fprintln(os.Stderr, "terminal-driver: stderr is not a TTY; using --frames")
		*frames = true
	}

	stepDur := 100 * time.Millisecond
	if *fast {
		stepDur = 20 * time.Millisecond
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

	// Same Init(Config) dialect as ordinary examples; advanced = Terminal field.
	out := evo.Init(evo.Config{
		Title:    "install-deps-advanced",
		Stdout:   os.Stderr,
		Stderr:   os.Stderr,
		Terminal: term,
		Debug:    evo.DebugConfig{Level: evo.LevelDebug},
		// Demo tuning: show spinners immediately.
		VisibilityDelay: evo.Delay(0),
		MaxFrameRate:    60,
		Isolated:        true,
	})

	os.Exit(out.Run(func(o *evo.Output) error {
		jobs := o.DisplayGroup("dependencies")
		discover := jobs.Task("discover")
		for _, phase := range []string{"reading lockfile", "resolving graph"} {
			discover.Doing(phase)
			time.Sleep(stepDur * 2)
		}
		discover.Done("%d packages", 12)

		download := jobs.Task("download")
		const total int64 = 4_000_000
		for i := 1; i <= 12; i++ {
			download.Bytes(total*int64(i)/12, total)
			time.Sleep(stepDur)
		}
		download.Done("4.0 MB")
		o.Task("registry").Done()
		return nil
	}))
}

// frameLog prints each redraw as a numbered scrubable frame (no in-place ANSI).
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
func (f *frameLog) Sink() io.Writer     { return f.w }
func (f *frameLog) Columns() int        { return f.width }
func (f *frameLog) Rows() int           { return 24 }
func (f *frameLog) IsInteractive() bool { return true }
func (f *frameLog) ClearLive()          {}
func (f *frameLog) WriteDurable(line string) {
	_, _ = fmt.Fprintf(f.w, "  · durable: %s\n", line)
}
func (f *frameLog) WriteFinal(text string) {
	_, _ = fmt.Fprintf(f.w, "\n── final ──\n%s\n", text)
}
func (f *frameLog) WriteLive(text string) {
	f.n++
	_, _ = fmt.Fprintf(f.w, "\n── frame %d ──\n%s\n", f.n, text)
	if f.step {
		_, _ = fmt.Fprint(f.w, "\n[Enter] next frame, or q+Enter to skip stepping… ")
		line, _ := f.in.ReadString('\n')
		if len(line) > 0 && (line[0] == 'q' || line[0] == 'Q') {
			f.step = false
		}
	}
}
